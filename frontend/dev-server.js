#!/usr/bin/env node
/*
 * Development server for the UI5 application.
 *
 * It serves the static files and proxies /api to the Go backend, so the
 * browser sees one origin and the CORS configuration of the backend is only
 * needed for genuinely cross-origin deployments.
 *
 * Node's standard library is enough — the frontend has no build step and no
 * npm dependencies, which keeps the toolchain to "open a browser".
 */
const http = require("http");
const fs = require("fs");
const path = require("path");

const PORT = Number(process.env.PORT || 8081);
const API_TARGET = process.env.API_TARGET || "http://127.0.0.1:8080";
const ROOT = __dirname;

const CONTENT_TYPES = {
	".html": "text/html; charset=utf-8",
	".js": "application/javascript; charset=utf-8",
	".json": "application/json; charset=utf-8",
	".xml": "application/xml; charset=utf-8",
	".css": "text/css; charset=utf-8",
	".properties": "text/plain; charset=utf-8",
	".png": "image/png",
	".svg": "image/svg+xml",
	".ico": "image/x-icon"
};

const target = new URL(API_TARGET);

function proxy(req, res) {
	const upstream = http.request({
		hostname: target.hostname,
		port: target.port || 80,
		path: req.url,
		method: req.method,
		headers: Object.assign({}, req.headers, { host: target.host })
	}, (upstreamRes) => {
		res.writeHead(upstreamRes.statusCode, upstreamRes.headers);
		upstreamRes.pipe(res);
	});

	upstream.on("error", (err) => {
		res.writeHead(502, { "Content-Type": "application/json" });
		res.end(JSON.stringify({
			success: false,
			error: { code: "E-GW-502", message: "The backend is not reachable: " + err.message }
		}));
	});

	req.pipe(upstream);
}

function serveStatic(req, res) {
	let relative = decodeURIComponent(req.url.split("?")[0]);
	if (relative === "/") {
		relative = "/index.html";
	}

	// Resolve inside the served root so a crafted path cannot escape it.
	const resolved = path.normalize(path.join(ROOT, relative));
	if (!resolved.startsWith(ROOT)) {
		res.writeHead(403).end("Forbidden");
		return;
	}

	fs.readFile(resolved, (err, content) => {
		if (err) {
			res.writeHead(404, { "Content-Type": "text/plain" }).end("Not found");
			return;
		}
		res.writeHead(200, {
			"Content-Type": CONTENT_TYPES[path.extname(resolved)] || "application/octet-stream",
			"Cache-Control": "no-cache"
		});
		res.end(content);
	});
}

http.createServer((req, res) => {
	if (req.url.startsWith("/api/")) {
		proxy(req, res);
		return;
	}
	serveStatic(req, res);
}).listen(PORT, () => {
	console.log(`UI5 frontend on http://localhost:${PORT} (API proxied to ${API_TARGET})`);
});
