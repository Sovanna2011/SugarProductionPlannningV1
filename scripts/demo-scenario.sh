#!/usr/bin/env bash
#
# Populates a running instance with a realistic working dataset, through the
# API, so every rule the server enforces applies to it.
#
# Seeding through the API rather than with SQL is deliberate: the data that
# lands in the database is data the business rules actually accepted, so what
# a tester sees on screen is reachable in normal use.
#
#   scripts/demo-scenario.sh [base-url]
#
# It is additive, not idempotent: it is meant for a freshly migrated database.
# Running it twice stacks a second campaign on top of the first, which can push
# a warehouse past its capacity and make the variance report meaningless. Pass
# --force to do it anyway.
#
# It follows the plant layout of docs/inventory-material-flow.mermaid:
# cane is milled into raw sugar, molasses and bagasse; raw sugar is remelted
# and refined; refined sugar is conditioned in the silo before it is packed,
# while white sugar goes straight to its warehouse.
set -uo pipefail

API="${1:-http://localhost:8080}/api/v1"
PASSWORD="${DEMO_USER_PASSWORD:-SugarPlanning#2026}"

step()  { printf '\n\033[1m%s\033[0m\n' "$1"; }
ok()    { printf '  ✓ %s\n' "$1"; }
warn()  { printf '  ! %s\n' "$1"; }

login() {
	curl -s -X POST "$API/auth/login" -H 'Content-Type: application/json' \
		-d "{\"username\":\"$1\",\"password\":\"$PASSWORD\"}" |
		jq -r '.data.tokens.accessToken'
}

# post <token> <path> <body> -> response body
post() { curl -s -X POST "$API$2" -H "Authorization: Bearer $1" \
	-H 'Content-Type: application/json' -d "$3"; }
get()  { curl -s "$API$2" -H "Authorization: Bearer $1"; }

day() { date -u -d "$1 day" +%Y-%m-%d; }

# --- sign in --------------------------------------------------------------

step "Signing in"
ADMIN=$(login admin); PLANNER=$(login planner)
APPROVER=$(login approver); OPERATOR=$(login operator)
if [ -z "$ADMIN" ] || [ "$ADMIN" = "null" ]; then
	echo "Cannot sign in — is the API running on ${API%/api/v1}?" >&2
	exit 1
fi
ok "admin, planner, approver and operator signed in"

# --- refuse to stack a second campaign on an already-seeded instance ------

ME_PROBE=$(get "$ADMIN" "/auth/me")
PROBE_COMPANY=$(echo "$ME_PROBE" | jq -r '.data.companies[]|select(.companyCode=="1000")|.companyId')
EXISTING=$(get "$ADMIN" "/actuals?companyId=$PROBE_COMPANY&size=1" | jq -r '.meta.total // 0')
if [ "$EXISTING" != "0" ] && [ "${2:-}" != "--force" ] && [ "${1:-}" != "--force" ]; then
	echo
	echo "  This instance already holds $EXISTING production documents." >&2
	echo "  Seeding again would stack a second campaign on top of the first." >&2
	echo "  Start from a fresh database, or pass --force to add anyway." >&2
	exit 1
fi

# --- resolve the master data ---------------------------------------------

step "Reading master data"
ME=$(get "$ADMIN" "/auth/me")
C1=$(echo "$ME" | jq -r '.data.companies[]|select(.companyCode=="1000")|.companyId')
C2=$(echo "$ME" | jq -r '.data.companies[]|select(.companyCode=="2000")|.companyId')

mat() { echo "$MATERIALS" | jq -r ".data[]|select(.materialCode==\"$1\")|.id"; }
wh()  { echo "$WAREHOUSES" | jq -r ".data[]|select(.warehouseCode==\"$1\")|.id"; }
line(){ echo "$LINES"      | jq -r ".data[]|select(.lineCode==\"$1\")|.id"; }
mvt() { echo "$MOVEMENTS"  | jq -r ".data[]|select(.movementCode==\"$1\")|.id"; }
prc() { echo "$PROCESSES"  | jq -r ".data[]|select(.processCode==\"$1\")|.id"; }

MATERIALS=$(get "$ADMIN" "/materials?size=100&companyId=$C1")
WAREHOUSES=$(get "$ADMIN" "/companies/$C1/warehouses?size=100")
LINES=$(get "$ADMIN" "/companies/$C1/production-lines?size=100")
MOVEMENTS=$(get "$ADMIN" "/movement-types?size=100&companyId=$C1")
PROCESSES=$(get "$ADMIN" "/processes?size=100&companyId=$C1")
SEASONS=$(get "$ADMIN" "/companies/$C1/seasons?size=100")

SEASON=$(echo "$SEASONS" | jq -r '.data[]|select(.status=="OPEN")|.id')
CANE=$(mat SUGARCANE); RAW=$(mat RAW_SUGAR); MOL=$(mat MOLASSES); BAG=$(mat BAGASSE)
REFINED=$(mat REFINED); WHITE=$(mat WHITE); ELEC=$(mat ELECTRICITY)
TON=$(echo "$MATERIALS" | jq -r '.data[]|select(.materialCode=="RAW_SUGAR")|.baseUomId')
MWH=$(echo "$MATERIALS" | jq -r '.data[]|select(.materialCode=="ELECTRICITY")|.baseUomId')

RW1=$(wh RW1); MT1=$(wh MT1); MT2=$(wh MT2); SILO=$(wh SILO1)
SW1=$(wh SW1); SW2=$(wh SW2); BAGYARD=$(wh BAGYARD)
L1=$(line LINE1); L2=$(line LINE2); L3=$(line LINE3); L5=$(line LINE5)
RECEIPT=$(mvt PRODUCTION_RECEIPT); ISSUE=$(mvt PRODUCTION_ISSUE)
REMELT_ISSUE=$(mvt REMELT_ISSUE)
MILLING=$(prc MILLING); REFINING=$(prc REFINING); PACKING=$(prc PACKING)
POWER=$(prc POWER); REMELT=$(prc REMELT)
ok "company 1000, open season, $(echo "$MATERIALS" | jq '.data|length') materials, $(echo "$WAREHOUSES" | jq '.data|length') storage locations"

# --- planning -------------------------------------------------------------

step "Creating the budget version and filling the planning matrix"

VERSION=$(post "$PLANNER" "/companies/$C1/seasons/$SEASON/planning-versions" \
	"{\"seasonId\":$SEASON,\"versionName\":\"Budget — crushing campaign\"}")
V1=$(echo "$VERSION" | jq -r '.data.id')
ok "version V$(echo "$VERSION" | jq -r '.data.versionNo') created as DRAFT"

# Plans one material across one or two production lines for a week.
#
# The week starts two days back so that the days already produced have both a
# plan and an actual — otherwise the variance report would be comparing one
# day of plan against three days of production.
plan_matrix() { # plan_matrix <material> <process> <lineA> <qtyA> [<lineB> <qtyB>]
	local material=$1 process=$2 lineA=$3 qtyA=$4 lineB=${5:-} qtyB=${6:-0}
	local rows="[]"

	for offset in -2 -1 0 1 2 3 4; do
		local date; date=$(day "$offset")
		# A deterministic wobble of ±5 %, which keeps every planned quantity
		# positive — a negative one is rejected, as it should be.
		local swing=$(( 95 + ((offset + 2) * 7) % 11 ))
		local values="[{\"productionLineId\":$lineA,\"quantity\":\"$(( qtyA * swing / 100 ))\"}"
		if [ -n "$lineB" ] && [ "$qtyB" -gt 0 ]; then
			values="$values,{\"productionLineId\":$lineB,\"quantity\":\"$(( qtyB * swing / 100 ))\"}"
		fi
		values="$values]"
		rows=$(jq -c --argjson v "$values" --arg d "$date" '. + [{planDate:$d, values:$v}]' <<<"$rows")
	done

	local processField="null"
	[ -n "$process" ] && processField="$process"

	local result; result=$(post "$PLANNER" "/plans/matrix?companyId=$C1" "$(jq -nc \
		--argjson season "$SEASON" --argjson version "$V1" --argjson mt "$RECEIPT" \
		--argjson material "$material" --argjson process "$processField" \
		--argjson uom "$TON" --arg from "$(day -2)" --arg to "$(day 4)" \
		--argjson rows "$rows" \
		'{seasonId:$season,versionId:$version,movementTypeId:$mt,materialId:$material,
		  processId:$process,uomId:$uom,dateFrom:$from,dateTo:$to,rows:$rows}')")

	# A rejected save must be visible; swallowing it would leave the tester
	# looking at a plan that silently lost a material.
	if [ "$(jq -r '.success' <<<"$result")" != "true" ]; then
		warn "planning material $material — $(jq -r '.error.code + ": " + .error.message' <<<"$result")"
		return 1
	fi
}

CANE=$(mat "SUGARCANE"); CANEYARD=$(wh "CANEYARD")

plan_matrix "$RAW"     "$MILLING"  "$L1" 3700 "$L2" 2500
plan_matrix "$MOL"     "$MILLING"  "$L1" 1350
plan_matrix "$BAG"     "$MILLING"  "$L1" 1600
plan_matrix "$REFINED" "$REFINING" "$L3" 1900
plan_matrix "$WHITE"   "$REFINING" "$L3" 660
ok "seven days planned (two back, four ahead) for raw sugar, molasses, bagasse, refined and white sugar"

# --- the cane that feeds all of it ----------------------------------------
#
# The harvest plan lives inside the same version as the production plan, so it
# has to be written before the version is submitted for approval.

step "Planning the cane supply"

GROWERS=$(get "$PLANNER" "/companies/$C1/growers?size=100")
grower() { echo "$GROWERS" | jq -r ".data[]|select(.growerCode==\"$1\")|.id"; }

EST1=$(grower "EST-01"); EST2=$(grower "EST-02")
OG1=$(grower "OG-101"); OG2=$(grower "OG-102"); OG3=$(grower "OG-103"); CT1=$(grower "CT-201")

# The mill needs roughly 6 200 t of cane a day to make the raw sugar above, and
# it comes from six growers: two own estates and four purchased suppliers.
harvest_rows="[]"
for offset in -2 -1 0 1 2 3 4; do
	date=$(day "$offset")
	swing=$(( 95 + ((offset + 2) * 7) % 11 ))
	values=$(jq -nc \
		--argjson e1 "$EST1" --argjson e2 "$EST2" --argjson o1 "$OG1" \
		--argjson o2 "$OG2" --argjson o3 "$OG3" --argjson c1 "$CT1" \
		--argjson s "$swing" \
		'[{growerId:$e1, plannedTons:((1500*$s/100)|tostring)},
		  {growerId:$e2, plannedTons:((1200*$s/100)|tostring)},
		  {growerId:$o1, plannedTons:((1100*$s/100)|tostring)},
		  {growerId:$o2, plannedTons:((1300*$s/100)|tostring)},
		  {growerId:$o3, plannedTons:(( 500*$s/100)|tostring)},
		  {growerId:$c1, plannedTons:(( 600*$s/100)|tostring)}]')
	harvest_rows=$(jq -c --argjson v "$values" --arg d "$date" \
		'. + [{planDate:$d, values:$v}]' <<<"$harvest_rows")
done

HARVEST=$(post "$PLANNER" "/harvest-plans/matrix?companyId=$C1" "$(jq -nc \
	--argjson version "$V1" --argjson uom "$TON" \
	--arg from "$(day -2)" --arg to "$(day 4)" --argjson rows "$harvest_rows" \
	'{versionId:$version,uomId:$uom,dateFrom:$from,dateTo:$to,rows:$rows}')")
if [ "$(jq -r '.success' <<<"$HARVEST")" = "true" ]; then
	ok "seven days of harvest planned across two own estates and four purchased suppliers"
else
	warn "planning the harvest — $(jq -r '.error.code + \": \" + .error.message' <<<"$HARVEST")"
fi

step "Taking the budget through approval"
post "$PLANNER"  "/planning-versions/$V1/submit?companyId=$C1"  '{}' >/dev/null
APPROVED=$(post "$APPROVER" "/planning-versions/$V1/approve?companyId=$C1" '{}')
ok "submitted by the planner, approved by the approver — status $(echo "$APPROVED" | jq -r '.data.status')"

step "Copying the budget into a working forecast"
COPY=$(post "$PLANNER" "/planning-versions/$V1/copy?companyId=$C1" \
	'{"versionName":"Forecast — working copy"}')
V2=$(echo "$COPY" | jq -r '.data.id')
ok "V$(echo "$COPY" | jq -r '.data.versionNo') is an independent snapshot of V1, left in DRAFT to edit"

# --- cane deliveries ------------------------------------------------------
#
# The actual against the harvest plan. Each posted ticket writes an ordinary
# inventory movement into the cane yard, so the cane obeys the same stock
# rules as everything else.

step "Preparing the weighbridge"

VARIETIES=$(get "$OPERATOR" "/cane-varieties?size=100&companyId=$C1")
K88=$(echo "$VARIETIES" | jq -r '.data[]|select(.varietyCode=="K88-92")|.id')

# ticket <grower> <date> <gross> <tare> <ccs> -> delivery id
ticket() {
	local grower=$1 date=$2 gross=$3 tare=$4 ccs=$5
	local body result
	body=$(jq -nc --argjson g "$grower" --arg d "$date" --argjson w "$CANEYARD" \
		--argjson v "$K88" --arg gross "$gross" --arg tare "$tare" --arg ccs "$ccs" \
		'{deliveryDate:$d, growerId:$g, warehouseId:$w, varietyId:$v,
		  grossTons:$gross, tareTons:$tare, ccsPct:$ccs, vehicleNo:"1AB-2345"}')
	result=$(post "$OPERATOR" "/cane-deliveries?companyId=$C1" "$body")
	if [ "$(jq -r '.success' <<<"$result")" != "true" ]; then
		warn "cane ticket — $(jq -r '.error.code + \": \" + .error.message' <<<"$result")"
		return 1
	fi
	jq -r '.data.id' <<<"$result"
}

# A ticket here stands for a consolidated day-lot rather than a single lorry:
# a real yard weighs hundreds of loads a day, and seeding those one by one
# would bury the screens without showing anything more. Three lots per grower
# per day land the delivered tonnage near the planned tonnage, so the variance
# a tester sees is the believable few per cent rather than an artefact of how
# much demo data there was patience for.
POSTED_TICKETS=0

# weigh_in <date> — one day of cane across the six growers.
weigh_in() {
	local date=$1 offset=$2
	for entry in "$EST1 500 13.4" "$EST2 400 13.1" "$OG1 365 13.9" \
	             "$OG2 430 14.2" "$OG3 165 12.6" "$CT1 200 13.0"; do
		set -- $entry
		for lot in 1 2 3; do
			# a deterministic wobble per lot, so no two loads are identical
			gross=$(( $2 + (lot * 7 + offset * 3) ))
			id=$(ticket "$1" "$date" "$gross" 14 "$3") || continue
			result=$(post "$OPERATOR" "/cane-deliveries/$id/post?companyId=$C1" '{}')
			if [ "$(jq -r '.data.postingStatus // empty' <<<"$result")" = "POSTED" ]; then
				POSTED_TICKETS=$((POSTED_TICKETS + 1))
			else
				# A silently refused posting would leave the tester looking at a
				# cane variance that is an artefact of this script, not of the data.
				warn "ticket refused — $(jq -r '.error.code + ": " + .error.message' <<<"$result")"
			fi
		done
	done
}



# --- actual production ----------------------------------------------------

step "Recording and posting actual production"

# document <token> <movementType> <date> <line> <description> <items-json>
document() {
	local mt=$1 date=$2 lineId=$3 description=$4 items=$5
	local lineField="null"; [ -n "$lineId" ] && lineField="$lineId"

	local created; created=$(post "$OPERATOR" "/actuals?companyId=$C1" "$(jq -nc \
		--argjson mt "$mt" --arg d "$date" --argjson line "$lineField" \
		--arg desc "$description" --argjson items "$items" \
		'{movementTypeId:$mt,postingDate:$d,productionLineId:$line,description:$desc,items:$items}')")

	local id; id=$(echo "$created" | jq -r '.data.id // empty')
	if [ -z "$id" ]; then
		warn "$description — $(echo "$created" | jq -r '.error.code + ": " + .error.message')"
		return 1
	fi
	echo "$id"
}

item() { # item <material> <warehouse|""> <qty> <uom> <process|""> <line|"">
	jq -nc --argjson m "$1" --argjson w "${2:-null}" --arg q "$3" --argjson u "$4" \
		--argjson p "${5:-null}" --argjson l "${6:-null}" --arg d "$DATE" \
		'{actualDate:$d,materialId:$m,warehouseId:$w,quantity:$q,uomId:$u,processId:$p,productionLineId:$l}'
}

post_document() {
	local id=$1 label=$2
	local result; result=$(post "$OPERATOR" "/actuals/$id/post?companyId=$C1" '{}')
	local status; status=$(echo "$result" | jq -r '.data.postingStatus // empty')
	if [ "$status" = "POSTED" ]; then
		ok "$label posted as $(echo "$result" | jq -r '.data.documentNo')"
	else
		warn "$label refused — $(echo "$result" | jq -r '.error.code + ": " + .error.message')"
	fi
}

# Three days of milling, each a little off plan so the variance is real.
for offset in -2 -1 0; do
	DATE=$(day "$offset")
	raw1=$(( 3600 + offset * 120 )); raw2=$(( 2400 + offset * 90 ))

	# The bridge first: the mill can only issue cane that has been weighed in.
	weigh_in "$DATE" "$offset"

	items=$(jq -nc --argjson a "$(item "$RAW" "$RW1" "$raw1" "$TON" "$MILLING" "$L1")" \
		--argjson b "$(item "$RAW" "$RW1" "$raw2" "$TON" "$MILLING" "$L2")" \
		--argjson c "$(item "$MOL" "$MT1" "1320" "$TON" "$MILLING" "$L1")" \
		--argjson d "$(item "$BAG" "$BAGYARD" "1580" "$TON" "$MILLING" "$L1")" \
		'[$a,$b,$c,$d]')

	if id=$(document "$RECEIPT" "$DATE" "$L1" "Milling output $DATE" "$items"); then
		post_document "$id" "milling $DATE"
	fi

	# Milling consumes the cane that was weighed in. Without this the yard
	# would only ever fill, and its capacity would be the thing that stopped
	# the demo rather than anything a tester was meant to see.
	if id=$(document "$ISSUE" "$DATE" "$L1" "Cane to the mill $DATE" \
		"$(jq -nc --argjson a "$(item "$CANE" "$CANEYARD" "3900" "$TON" "$MILLING" "$L1")" '[$a]')"); then
		post_document "$id" "cane issued to milling $DATE"
	fi
done

ok "$POSTED_TICKETS cane tickets posted over three days — priced where the cane was bought"

# One ticket is deliberately left as a draft so a tester can post it themselves
# and watch the yard move.
DRAFT_TICKET=$(ticket "$OG2" "$(day 0)" 435 14 14.0)
ok "one cane ticket left in DRAFT for the tester to post"

# The power plant burns bagasse and generates electricity, which is a
# production quantity and never warehouse stock.
DATE=$(day 0)
if id=$(document "$ISSUE" "$DATE" "" "Bagasse to the power plant" \
	"$(jq -nc --argjson a "$(item "$BAG" "$BAGYARD" "2400" "$TON" "$POWER" "")" '[$a]')"); then
	post_document "$id" "bagasse issue"
fi
if id=$(document "$RECEIPT" "$DATE" "" "Electricity generated" \
	"$(jq -nc --argjson a "$(item "$ELEC" "null" "310" "$MWH" "$POWER" "")" '[$a]')"); then
	post_document "$id" "electricity (not stock managed)"
fi

# Raw sugar is remelted, then refined.
if id=$(document "$REMELT_ISSUE" "$DATE" "$L3" "Raw sugar to remelt" \
	"$(jq -nc --argjson a "$(item "$RAW" "$RW1" "2600" "$TON" "$REMELT" "$L3")" '[$a]')"); then
	post_document "$id" "remelt issue"
fi

# Refined sugar must be received into the conditioning silo first.
if id=$(document "$RECEIPT" "$DATE" "$L3" "Refined sugar into the condition silo" \
	"$(jq -nc --argjson a "$(item "$REFINED" "$SILO" "1850" "$TON" "$REFINING" "$L3")" '[$a]')"); then
	post_document "$id" "refined into the silo"
fi

# White sugar goes direct — it never enters the silo.
if id=$(document "$RECEIPT" "$DATE" "$L3" "White sugar direct to its warehouse" \
	"$(jq -nc --argjson a "$(item "$WHITE" "$SW2" "640" "$TON" "$REFINING" "$L3")" '[$a]')"); then
	post_document "$id" "white sugar direct"
fi

# Conditioned sugar leaves the silo and is packed into its warehouse.
if id=$(document "$ISSUE" "$DATE" "$L5" "Conditioned sugar out of the silo" \
	"$(jq -nc --argjson a "$(item "$REFINED" "$SILO" "700" "$TON" "$PACKING" "$L5")" '[$a]')"); then
	post_document "$id" "silo issue to packing"
fi
if id=$(document "$RECEIPT" "$DATE" "$L5" "Packed refined sugar" \
	"$(jq -nc --argjson a "$(item "$REFINED" "$SW1" "690" "$TON" "$PACKING" "$L5")" '[$a]')"); then
	post_document "$id" "packed refined sugar"
fi

# --- inventory ------------------------------------------------------------

step "Moving stock between tanks"
TRANSFER=$(post "$OPERATOR" "/inventory/transfers?companyId=$C1" "$(jq -nc \
	--argjson from "$MT1" --argjson to "$MT2" --argjson m "$MOL" \
	--argjson u "$TON" --arg d "$(day 0)" \
	'{fromWarehouseId:$from,toWarehouseId:$to,materialId:$m,quantity:"900",uomId:$u,
	  transactionDate:$d,remark:"Rebalancing the tank farm"}')")
if [ "$(echo "$TRANSFER" | jq -r '.success')" = "true" ]; then
	ok "900 t of molasses moved from MT1 to MT2 as $(echo "$TRANSFER" | jq -r '.data[0].documentNo')"
else
	warn "transfer refused — $(echo "$TRANSFER" | jq -r '.error.code')"
fi

step "Leaving work in progress for the tester"
DATE=$(day 0)
if id=$(document "$RECEIPT" "$DATE" "$L2" "Evening shift — not yet posted" \
	"$(jq -nc --argjson a "$(item "$RAW" "$RW1" "2450" "$TON" "$MILLING" "$L2")" '[$a]')"); then
	ok "one draft document left unposted, ready to be posted from the UI"
fi

# --- a second company so consolidated reporting has something to show -----

step "Planning a second company"
SEASON2=$(get "$ADMIN" "/companies/$C2/seasons?size=100" | jq -r '.data[]|select(.status=="OPEN")|.id')
V3=$(post "$ADMIN" "/companies/$C2/seasons/$SEASON2/planning-versions" \
	"{\"seasonId\":$SEASON2,\"versionName\":\"Budget — north plant\"}" | jq -r '.data.id')
LINES2=$(get "$ADMIN" "/companies/$C2/production-lines?size=100")
L2A=$(echo "$LINES2" | jq -r '.data[]|select(.lineCode=="LINE1")|.id')

rows2=$(jq -nc --arg d0 "$(day 0)" --arg d1 "$(day 1)" --argjson l "$L2A" \
	'[{planDate:$d0,values:[{productionLineId:$l,quantity:"2800"}]},
	  {planDate:$d1,values:[{productionLineId:$l,quantity:"2900"}]}]')
post "$ADMIN" "/plans/matrix?companyId=$C2" "$(jq -nc \
	--argjson s "$SEASON2" --argjson v "$V3" --argjson mt "$RECEIPT" --argjson m "$RAW" \
	--argjson p "$MILLING" --argjson u "$TON" --arg from "$(day 0)" --arg to "$(day 1)" \
	--argjson rows "$rows2" \
	'{seasonId:$s,versionId:$v,movementTypeId:$mt,materialId:$m,processId:$p,uomId:$u,
	  dateFrom:$from,dateTo:$to,rows:$rows}')" >/dev/null
post "$ADMIN" "/planning-versions/$V3/submit?companyId=$C2" '{}' >/dev/null
post "$ADMIN" "/planning-versions/$V3/approve?companyId=$C2" '{}' >/dev/null
ok "company 2000 has an approved budget, so the consolidated report spans two companies"

# --- what the tester will see --------------------------------------------

step "Result"
BAL=$(get "$OPERATOR" "/inventory/balances?companyId=$C1")
printf '  stock now held\n'
echo "$BAL" | jq -r '.data[] | "    \(.warehouseCode|.[0:8]) \(.materialCode|.[0:12])\t\(.closingQty) \(.uomCode)"'

CAP=$(get "$OPERATOR" "/inventory/capacity?companyId=$C1")
printf '\n  capacity\n'
echo "$CAP" | jq -r '.data[] | select(.utilisationPct != null) |
	"    \(.warehouseCode)\t\(.utilisationPct) %\t\(.trafficLight)"'

PVA=$(get "$PLANNER" "/reports/plan-vs-actual?companyId=$C1&seasonId=$SEASON&versionId=$V1&dateFrom=$(day -2)&dateTo=$(day 0)&groupBy=material")
printf '\n  plan vs actual for the three days that have been produced\n'
echo "$PVA" | jq -r '.data.lines[] |
	"    \(.groupKey)\tplan \(.planQty)\tactual \(.actualQty)\tvariance \(.variance) (\(.variancePct // "n/a"))"'

CANE=$(get "$PLANNER" "/reports/cane-plan-vs-actual?companyId=$C1&versionId=$V1&groupBy=supplyType&dateFrom=$(day -2)&dateTo=$(day 0)")
printf '\n  cane: planned harvest against what the bridge weighed\n'
echo "$CANE" | jq -r '.data[] |
	"    \(.groupLabel)\tplan \(.plannedTons)\tactual \(.actualTons)\tvariance \(.variancePct // "n/a")\tvalue \(.purchaseValue // "-")"'

printf '\n'
ok "the system is populated and ready for testing"
