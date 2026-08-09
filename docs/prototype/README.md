# The clickable prototype

`index.html` is a single self-contained page that mirrors the screens and the
business rules of the service. It exists because the real UI cannot always be
put in front of someone: the UI5 runtime loads from a CDN, the API needs a
database, and a tester with a link needs neither.

Open it in a browser — there is nothing to install and nothing to run.

## What it is, and what it is not

It **is** the rules: the variance formula and its `n/a`, the null-capacity rule,
the version status machine, four-eyes approval, the silo routing, the
negative-stock and capacity refusals, per-*(user, company)* permissions,
derived net weight, and the difference between purchased and own-estate cane.
Each is ported from the Go service so it behaves the same way, including the
error codes it refuses with.

It is **not** connected to anything. Nothing is saved; reloading starts over.
Where the prototype and the service disagree, the service is right.

## Verifying it

`verify.js` drives the page in a real browser and asserts that the rules it
claims to enforce actually fire — in both light and dark:

```bash
npm install playwright-core
node verify.js
```

35 checks, run twice for the two colour schemes. Two of them exist because a
bug got through everything else and was only caught by looking at a
screenshot: the line totals were computed from the wrong value, and every one
of them rendered zero while the day totals beside them were right.
