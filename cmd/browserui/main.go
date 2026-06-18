package main

import (
	"encoding/json"
	"flag"
	"html/template"
	"log"
	"net/http"
	"strings"
)

type pageData struct {
	BrokerURL template.JS
	Session   template.JS
	Stun      template.JS
	TargetURL template.JS
}

func main() {
	listen := flag.String("listen", "127.0.0.1:7777", "HTTP listen address for the local browser UI")
	brokerURL := flag.String("broker", "http://127.0.0.1:8080", "signaling broker URL")
	sessionID := flag.String("session", "browser-session", "shared signaling session id")
	stunServers := flag.String("stun", "stun:stun.l.google.com:19302", "comma-separated STUN URLs; empty disables external STUN")
	targetURL := flag.String("target-url", "", "configured proxy target URL hint for browser input normalization")
	flag.Parse()

	tmpl := template.Must(template.New("browserui").Parse(browserPage))
	data := pageData{
		BrokerURL: jsString(*brokerURL),
		Session:   jsString(*sessionID),
		Stun:      jsString(*stunServers),
		TargetURL: jsString(*targetURL),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("render browser UI: %v", err)
		}
	})

	log.Printf("browser UI listening on %s", displayURL(*listen))
	log.Printf("open the UI locally and connect to broker %s with session %q", *brokerURL, *sessionID)
	if strings.TrimSpace(*targetURL) != "" {
		log.Printf("browser UI target hint: %s", *targetURL)
	}
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
}

func jsString(value string) template.JS {
	encoded, err := json.Marshal(value)
	if err != nil {
		return template.JS(`""`)
	}
	return template.JS(encoded)
}

func displayURL(listen string) string {
	listen = strings.TrimSpace(listen)
	if strings.HasPrefix(listen, ":") {
		return "http://127.0.0.1" + listen
	}
	return "http://" + listen
}

const browserPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>WebRTC Proxy Viewer</title>
  <style>
    :root {
      color-scheme: light;
      --ink: #171717;
      --muted: #5f6368;
      --line: #d8dee4;
      --panel: #ffffff;
      --soft: #f6f8fa;
      --accent: #0b6bcb;
      --ok: #126b40;
      --warn: #9a6700;
      --bad: #b42318;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: var(--ink);
      background: #eef2f6;
    }
    .app {
      min-height: 100vh;
      display: grid;
      grid-template-rows: auto auto 1fr;
    }
    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding: 14px 18px;
      border-bottom: 1px solid var(--line);
      background: var(--panel);
    }
    h1 {
      margin: 0;
      font-size: 18px;
      font-weight: 650;
      letter-spacing: 0;
    }
    .status {
      display: inline-flex;
      align-items: center;
      min-height: 32px;
      padding: 0 10px;
      border: 1px solid var(--line);
      border-radius: 6px;
      background: var(--soft);
      color: var(--muted);
      font-size: 13px;
      white-space: nowrap;
    }
    .status.ok { color: var(--ok); border-color: #94d3ac; background: #f0fff5; }
    .status.warn { color: var(--warn); border-color: #d4a72c; background: #fff8c5; }
    .status.bad { color: var(--bad); border-color: #f1aeb5; background: #fff1f3; }
    .controls {
      display: grid;
      grid-template-columns: minmax(220px, 1fr) minmax(220px, 1fr) minmax(140px, 180px) minmax(160px, 240px) auto;
      gap: 10px;
      padding: 12px 18px;
      border-bottom: 1px solid var(--line);
      background: #fbfcfe;
    }
    label {
      display: grid;
      gap: 4px;
      font-size: 12px;
      color: var(--muted);
    }
    input {
      width: 100%;
      height: 34px;
      padding: 6px 9px;
      border: 1px solid var(--line);
      border-radius: 6px;
      color: var(--ink);
      background: var(--panel);
      font: inherit;
      font-size: 13px;
    }
    .buttons {
      display: flex;
      align-items: end;
      gap: 8px;
    }
    button {
      height: 34px;
      border: 1px solid #0958a8;
      border-radius: 6px;
      padding: 0 12px;
      background: var(--accent);
      color: white;
      font: inherit;
      font-size: 13px;
      cursor: pointer;
    }
    button.secondary {
      border-color: var(--line);
      background: var(--panel);
      color: var(--ink);
    }
    button:disabled {
      cursor: not-allowed;
      opacity: 0.55;
    }
    main {
      display: grid;
      grid-template-columns: minmax(0, 1fr) 360px;
      min-height: 0;
    }
    .viewer-shell {
      display: grid;
      grid-template-rows: auto 1fr;
      min-width: 0;
      min-height: 0;
      background: var(--panel);
    }
    .nav {
      display: grid;
      grid-template-columns: minmax(120px, 1fr) auto;
      gap: 8px;
      padding: 10px 12px;
      border-bottom: 1px solid var(--line);
      background: var(--soft);
    }
    #viewer {
      min-height: 0;
      overflow: auto;
      padding: 18px;
      background: white;
    }
    aside {
      min-width: 0;
      border-left: 1px solid var(--line);
      background: #101418;
      color: #d9e2ec;
      display: grid;
      grid-template-rows: auto 1fr;
    }
    aside h2 {
      margin: 0;
      padding: 12px 14px;
      font-size: 13px;
      font-weight: 650;
      border-bottom: 1px solid #2f3740;
    }
    #log {
      margin: 0;
      padding: 12px 14px;
      overflow: auto;
      white-space: pre-wrap;
      font: 12px/1.45 ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
    }
    .empty {
      color: var(--muted);
      max-width: 740px;
      line-height: 1.5;
    }
    .response-meta {
      margin-bottom: 12px;
      padding: 10px 12px;
      border: 1px solid var(--line);
      border-radius: 6px;
      background: var(--soft);
      color: var(--muted);
      font-size: 13px;
      overflow-wrap: anywhere;
    }
    .plain-response {
      white-space: pre-wrap;
      font: 13px/1.5 ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
    }
    .empty-render {
      margin: 12px 0;
      padding: 12px;
      border: 1px solid #f0c36d;
      border-radius: 6px;
      background: #fff8dc;
      color: #6f4e00;
      font-size: 13px;
      line-height: 1.5;
    }
    .source-preview {
      margin-top: 10px;
      white-space: pre-wrap;
      overflow: auto;
      max-height: 360px;
      padding: 10px;
      border: 1px solid var(--line);
      border-radius: 6px;
      background: #f6f8fa;
      color: #24292f;
      font: 12px/1.45 ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
    }
    @media (max-width: 920px) {
      .controls { grid-template-columns: 1fr; }
      .buttons { align-items: stretch; }
      main { grid-template-columns: 1fr; grid-template-rows: minmax(360px, 1fr) 260px; }
      aside { border-left: 0; border-top: 1px solid var(--line); }
    }
  </style>
</head>
<body>
  <div class="app">
    <header>
      <h1>WebRTC Proxy Viewer</h1>
      <div id="status" class="status">Disconnected</div>
    </header>

    <section class="controls" aria-label="Connection controls">
      <label>Broker URL
        <input id="broker" autocomplete="off">
      </label>
      <label>Target URL hint
        <input id="target" autocomplete="off">
      </label>
      <label>Session
        <input id="session" autocomplete="off">
      </label>
      <label>STUN servers
        <input id="stun" autocomplete="off">
      </label>
      <div class="buttons">
        <button id="connect">Connect</button>
        <button id="disconnect" class="secondary" disabled>Disconnect</button>
      </div>
    </section>

    <main>
      <section class="viewer-shell" aria-label="Proxy viewer">
        <div class="nav">
          <input id="path" value="/" autocomplete="off" aria-label="Target path or URL">
          <button id="go" disabled>Go</button>
        </div>
        <div id="viewer">
          <p class="empty">Connect to the proxy session, then request <code>/</code>, <code>/robots.txt</code>, or a URL under the configured target such as <code>https://example.com/robots.txt</code>. Returned HTML is sanitized before rendering so scripts, forms, and external assets do not load directly from this device.</p>
        </div>
      </section>
      <aside aria-label="Connection log">
        <h2>Log</h2>
        <pre id="log"></pre>
      </aside>
    </main>
  </div>

  <script>
    "use strict";

    const config = {
      brokerUrl: {{ .BrokerURL }},
      session: {{ .Session }},
      stun: {{ .Stun }},
      targetUrl: {{ .TargetURL }}
    };
    const requestType = "LAB_PROXY_REQUEST";
    const responseChunkType = "LAB_PROXY_RESPONSE_CHUNK";

    const brokerInput = document.getElementById("broker");
    const targetInput = document.getElementById("target");
    const sessionInput = document.getElementById("session");
    const stunInput = document.getElementById("stun");
    const pathInput = document.getElementById("path");
    const connectButton = document.getElementById("connect");
    const disconnectButton = document.getElementById("disconnect");
    const goButton = document.getElementById("go");
    const statusBox = document.getElementById("status");
    const logBox = document.getElementById("log");
    const viewer = document.getElementById("viewer");

    let peer = null;
    let channel = null;
    let requestCount = 0;
    let shadow = null;
    const pendingResponses = new Map();

    brokerInput.value = config.brokerUrl;
    targetInput.value = config.targetUrl;
    sessionInput.value = config.session;
    stunInput.value = config.stun;

    connectButton.addEventListener("click", connect);
    disconnectButton.addEventListener("click", disconnect);
    goButton.addEventListener("click", function () { requestPath(pathInput.value); });
    pathInput.addEventListener("keydown", function (event) {
      if (event.key === "Enter" && !goButton.disabled) {
        requestPath(pathInput.value);
      }
    });

    function setStatus(text, kind) {
      statusBox.textContent = text;
      statusBox.className = "status" + (kind ? " " + kind : "");
    }

    function logLine(text) {
      const stamp = new Date().toLocaleTimeString();
      logBox.textContent += "[" + stamp + "] " + text + "\n";
      logBox.scrollTop = logBox.scrollHeight;
    }

    function signalURL(kind) {
      const root = brokerInput.value.trim().replace(/\/+$/, "");
      return root + "/sessions/" + encodeURIComponent(sessionInput.value.trim()) + "/" + encodeURIComponent(kind);
    }

    function iceServers() {
      const urls = stunInput.value.split(",").map(function (part) {
        return part.trim();
      }).filter(Boolean);
      return urls.length ? [{ urls: urls }] : [];
    }

    async function connect() {
      disconnect();
      setStatus("Creating WebRTC offer", "warn");
      connectButton.disabled = true;
      logLine("creating browser RTCPeerConnection");

      try {
        peer = new RTCPeerConnection({ iceServers: iceServers() });
        peer.addEventListener("iceconnectionstatechange", function () {
          logLine("ICE connection state: " + peer.iceConnectionState);
          if (peer.iceConnectionState === "connected" || peer.iceConnectionState === "completed") {
            setStatus("WebRTC connected", "ok");
          }
        });
        peer.addEventListener("connectionstatechange", function () {
          logLine("peer connection state: " + peer.connectionState);
        });

        channel = peer.createDataChannel("lab-proxy");
        wireDataChannel(channel);

        const offer = await peer.createOffer();
        await peer.setLocalDescription(offer);
        await waitForIceGathering(peer);
        await postSignal("offer", peer.localDescription);
        logLine("posted SDP offer to broker");

        setStatus("Waiting for proxy answer", "warn");
        const answer = await pollSignal("answer", 120000);
        await peer.setRemoteDescription(answer);
        logLine("applied SDP answer from proxy");

        disconnectButton.disabled = false;
      } catch (error) {
        logLine("connect failed: " + error.message);
        setStatus("Connection failed", "bad");
        connectButton.disabled = false;
        disconnectButton.disabled = true;
        goButton.disabled = true;
      }
    }

    function disconnect() {
      if (channel) {
        try { channel.close(); } catch (_) {}
      }
      if (peer) {
        try { peer.close(); } catch (_) {}
      }
      channel = null;
      peer = null;
      connectButton.disabled = false;
      disconnectButton.disabled = true;
      goButton.disabled = true;
      if (statusBox.textContent !== "Disconnected") {
        setStatus("Disconnected", "");
      }
    }

    function wireDataChannel(dc) {
      dc.binaryType = "arraybuffer";
      dc.addEventListener("open", function () {
        logLine("proxy data channel \"lab-proxy\" open");
        setStatus("DataChannel open", "ok");
        goButton.disabled = false;
        requestPath(pathInput.value);
      });
      dc.addEventListener("close", function () {
        logLine("proxy data channel closed");
        goButton.disabled = true;
      });
      dc.addEventListener("message", function (event) {
        decodeMessage(event.data).then(handleProxyResponse).catch(function (error) {
          logLine("decode proxy response failed: " + error.message);
        });
      });
    }

    async function decodeMessage(data) {
      if (typeof data === "string") {
        return data;
      }
      if (data instanceof ArrayBuffer) {
        return new TextDecoder().decode(data);
      }
      if (data instanceof Blob) {
        return data.text();
      }
      return String(data);
    }

    async function waitForIceGathering(pc) {
      if (pc.iceGatheringState === "complete") {
        return;
      }
      await new Promise(function (resolve) {
        pc.addEventListener("icegatheringstatechange", function onStateChange() {
          if (pc.iceGatheringState === "complete") {
            pc.removeEventListener("icegatheringstatechange", onStateChange);
            resolve();
          }
        });
      });
    }

    async function postSignal(kind, description) {
      const response = await fetch(signalURL(kind), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ type: description.type, sdp: description.sdp })
      });
      if (!response.ok) {
        throw new Error("broker returned " + response.status + " for " + kind);
      }
    }

    async function pollSignal(kind, timeoutMs) {
      const deadline = Date.now() + timeoutMs;
      while (Date.now() < deadline) {
        const response = await fetch(signalURL(kind), { method: "GET" });
        if (response.status === 200) {
          return response.json();
        }
        if (response.status !== 404) {
          throw new Error("broker returned " + response.status + " while polling " + kind);
        }
        await sleep(1000);
      }
      throw new Error("timed out waiting for " + kind);
    }

    function sleep(ms) {
      return new Promise(function (resolve) { window.setTimeout(resolve, ms); });
    }

    function requestPath(rawPath) {
      if (!channel || channel.readyState !== "open") {
        logLine("cannot request path before DataChannel is open");
        return;
      }

      let path = normalizePath(rawPath);
      if (!path) {
        return;
      }
      pathInput.value = path;

      requestCount += 1;
      const requestID = "browser-" + String(requestCount).padStart(3, "0");
      const payload = {
        type: requestType,
        id: requestID,
        method: "GET",
        path: path
      };

      channel.send(JSON.stringify(payload));
      logLine("sent proxy request id=" + requestID + " path=" + path);
    }

    function normalizePath(value) {
      let path = String(value || "").trim();
      if (!path) {
        return "/";
      }

      const target = targetBaseURL();
      const inputURL = parseURLInput(path, target);
      if (inputURL) {
        if (!target) {
          logLine("set the target URL hint before entering a full URL");
          return "";
        }
        if (inputURL.origin !== target.origin) {
          logLine("blocked " + inputURL.origin + "; proxy session is bounded to " + target.origin);
          return "";
        }

        const normalized = targetRelativePath(inputURL, target);
        if (!normalized) {
          logLine("blocked " + inputURL.pathname + "; URL path is outside the configured target base path");
          return "";
        }
        logLine("normalized " + path + " to " + (normalized || "/"));
        return normalized || "/";
      }

      if (!path.startsWith("/")) {
        path = "/" + path;
      }
      return path;
    }

    function targetRelativePath(inputURL, targetURL) {
      const basePath = normalizedBasePath(targetURL.pathname);
      if (basePath && inputURL.pathname !== basePath && !inputURL.pathname.startsWith(basePath + "/")) {
        return "";
      }

      let path = basePath ? inputURL.pathname.slice(basePath.length) : inputURL.pathname;
      if (!path) {
        path = "/";
      }
      if (!path.startsWith("/")) {
        path = "/" + path;
      }
      return path + inputURL.search;
    }

    function normalizedBasePath(path) {
      path = String(path || "").replace(/\/+$/, "");
      return path === "" ? "" : path;
    }

    function targetBaseURL() {
      const raw = targetInput.value.trim();
      if (!raw) {
        return null;
      }

      try {
        return new URL(raw.indexOf("://") === -1 ? "https://" + raw : raw);
      } catch (error) {
        logLine("target URL hint is invalid: " + raw);
        return null;
      }
    }

    function parseURLInput(value, target) {
      if (value.startsWith("http://") || value.startsWith("https://")) {
        try {
          return new URL(value);
        } catch (_) {
          return null;
        }
      }

      if (value.startsWith("//")) {
        if (!target) {
          return null;
        }
        try {
          return new URL(target.protocol + value);
        } catch (_) {
          return null;
        }
      }

      if (looksLikeHostPath(value)) {
        try {
          return new URL("https://" + value);
        } catch (_) {
          return null;
        }
      }

      return null;
    }

    function looksLikeHostPath(value) {
      const hostPart = value.split(/[/?#]/, 1)[0].toLowerCase();
      return hostPart === "localhost" || hostPart.indexOf(".") !== -1 || /^\d{1,3}(\.\d{1,3}){3}$/.test(hostPart);
    }

    function handleProxyResponse(raw) {
      let response;
      try {
        response = JSON.parse(raw);
      } catch (error) {
        logLine("invalid proxy response: " + raw);
        return;
      }

      if (response.type === responseChunkType) {
        handleProxyResponseChunk(response);
        return;
      }

      if (response.error) {
        logLine("proxy response id=" + response.id + " error=" + response.error);
        renderError(response.error);
        return;
      }

      logLine("proxy response id=" + response.id + " status=" + response.status + " bytes=" + response.bytes + " target=" + response.target);
      renderResponse(response);
    }

    function handleProxyResponseChunk(chunk) {
      if (!chunk.id || !Number.isInteger(chunk.chunk_total) || !Number.isInteger(chunk.chunk_index)) {
        logLine("invalid proxy response chunk metadata");
        return;
      }
      if (chunk.chunk_total <= 0 || chunk.chunk_index < 0 || chunk.chunk_index >= chunk.chunk_total) {
        logLine("invalid proxy response chunk index for id=" + chunk.id);
        return;
      }
      if (chunk.body_encoding !== "base64") {
        logLine("unsupported proxy response chunk encoding for id=" + chunk.id + ": " + chunk.body_encoding);
        return;
      }

      let state = pendingResponses.get(chunk.id);
      if (!state) {
        state = {
          response: chunk,
          chunks: new Array(chunk.chunk_total),
          received: 0
        };
        pendingResponses.set(chunk.id, state);
      }
      if (state.chunks.length !== chunk.chunk_total) {
        pendingResponses.delete(chunk.id);
        logLine("discarded proxy response id=" + chunk.id + " because chunk total changed");
        return;
      }
      if (!state.chunks[chunk.chunk_index]) {
        state.received += 1;
      }
      state.chunks[chunk.chunk_index] = chunk.body_chunk || "";

      logLine("proxy response chunk id=" + chunk.id + " chunk=" + (chunk.chunk_index + 1) + "/" + chunk.chunk_total);
      if (state.received < state.chunks.length) {
        return;
      }

      try {
        const body = decodeChunkedBody(state.chunks);
        const complete = Object.assign({}, state.response, {
          type: "LAB_PROXY_RESPONSE",
          body: body,
          body_chunk: "",
          body_encoding: "",
          chunk_index: 0,
          chunk_total: 0
        });
        pendingResponses.delete(chunk.id);
        logLine("proxy response id=" + complete.id + " status=" + complete.status + " bytes=" + complete.bytes + " target=" + complete.target);
        renderResponse(complete);
      } catch (error) {
        pendingResponses.delete(chunk.id);
        logLine("failed to assemble proxy response id=" + chunk.id + ": " + error.message);
      }
    }

    function decodeChunkedBody(chunks) {
      const parts = chunks.map(function (encoded, index) {
        if (!encoded) {
          throw new Error("missing chunk " + (index + 1));
        }
        return base64ToBytes(encoded);
      });

      const total = parts.reduce(function (sum, part) {
        return sum + part.length;
      }, 0);
      const merged = new Uint8Array(total);
      let offset = 0;
      parts.forEach(function (part) {
        merged.set(part, offset);
        offset += part.length;
      });
      return new TextDecoder().decode(merged);
    }

    function base64ToBytes(encoded) {
      const binary = atob(encoded);
      const out = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i += 1) {
        out[i] = binary.charCodeAt(i);
      }
      return out;
    }

    function renderResponse(response) {
      const body = response.body || response.body_preview || "";
      const contentType = String(response.content_type || "").toLowerCase();
      const looksHTML = contentType.indexOf("text/html") !== -1 || /^\s*<!doctype|\s*<html[\s>]/i.test(body);

      viewer.innerHTML = "";
      const meta = document.createElement("div");
      meta.className = "response-meta";
      meta.textContent = "Status " + response.status + " from " + response.target + (response.truncated ? " (body truncated by proxy max-body limit)" : "");
      viewer.appendChild(meta);

      if (looksHTML) {
        renderHTML(body, response.target);
      } else {
        const pre = document.createElement("pre");
        pre.className = "plain-response";
        pre.textContent = body;
        viewer.appendChild(pre);
      }
    }

    function renderError(message) {
      viewer.innerHTML = "";
      const box = document.createElement("div");
      box.className = "response-meta";
      box.textContent = "Proxy error: " + message;
      viewer.appendChild(box);
    }

    function renderHTML(html, targetURL) {
      const host = document.createElement("div");
      viewer.appendChild(host);
      shadow = host.attachShadow({ mode: "open" });

      const parser = new DOMParser();
      const doc = parser.parseFromString(html, "text/html");
      sanitizeDocument(doc, targetURL);

      const style = document.createElement("style");
      style.textContent = [
        ":host { all: initial; color: #171717; font-family: Arial, Helvetica, sans-serif; line-height: 1.45; }",
        "* { box-sizing: border-box; max-width: 100%; }",
        "body, main, section, article, div { max-width: 100%; }",
        "a[data-proxy-path] { color: #0b6bcb; cursor: pointer; text-decoration: underline; }",
        "a.disabled-link { color: #6e7781; cursor: not-allowed; text-decoration: line-through; }",
        "img, picture, video, audio, canvas, svg { display: none !important; }",
        ".lab-note { margin: 0 0 12px; padding: 10px 12px; border: 1px solid #d8dee4; border-radius: 6px; background: #f6f8fa; color: #5f6368; font: 13px Arial, Helvetica, sans-serif; }",
        ".empty-render { margin: 12px 0; padding: 12px; border: 1px solid #f0c36d; border-radius: 6px; background: #fff8dc; color: #6f4e00; font: 13px/1.5 Arial, Helvetica, sans-serif; }",
        ".source-preview { margin-top: 10px; white-space: pre-wrap; overflow: auto; max-height: 360px; padding: 10px; border: 1px solid #d8dee4; border-radius: 6px; background: #f6f8fa; color: #24292f; font: 12px/1.45 ui-monospace, SFMono-Regular, Consolas, 'Liberation Mono', monospace; }"
      ].join("\n");
      shadow.appendChild(style);

      const note = document.createElement("div");
      note.className = "lab-note";
      note.textContent = "Sanitized HTML view. Scripts, forms, external assets, and cross-site links are disabled so this device does not directly browse the target site.";
      shadow.appendChild(note);

      const content = document.createElement("div");
      content.innerHTML = doc.body ? doc.body.innerHTML : doc.documentElement.innerHTML;
      shadow.appendChild(content);

      shadow.querySelectorAll("a[data-proxy-path]").forEach(function (anchor) {
        anchor.addEventListener("click", function (event) {
          event.preventDefault();
          requestPath(anchor.getAttribute("data-proxy-path"));
        });
      });

      showFallbackIfEmpty(content, html);
    }

    function sanitizeDocument(doc, targetURL) {
      doc.querySelectorAll("script, iframe, object, embed, link, meta, form, style").forEach(function (element) {
        element.remove();
      });

      doc.querySelectorAll("*").forEach(function (element) {
        Array.from(element.attributes).forEach(function (attribute) {
          const name = attribute.name.toLowerCase();
          if (name.startsWith("on") || ["src", "srcset", "poster", "ping", "action", "integrity", "style", "xlink:href"].indexOf(name) !== -1) {
            element.removeAttribute(attribute.name);
          }
          if (["hidden", "inert", "aria-hidden"].indexOf(name) !== -1) {
            element.removeAttribute(attribute.name);
          }
          if (name === "href" && element.tagName.toLowerCase() !== "a") {
            element.removeAttribute(attribute.name);
          }
        });
      });

      let baseURL;
      try {
        baseURL = new URL(targetURL);
      } catch (_) {
        baseURL = null;
      }

      doc.querySelectorAll("a[href]").forEach(function (anchor) {
        const raw = anchor.getAttribute("href");
        anchor.removeAttribute("target");
        anchor.removeAttribute("rel");

        if (!raw || raw.startsWith("#") || !baseURL) {
          anchor.setAttribute("href", "#");
          return;
        }

        try {
          const linked = new URL(raw, targetURL);
          if (linked.origin === baseURL.origin) {
            anchor.setAttribute("href", "#");
            anchor.setAttribute("data-proxy-path", linked.pathname + linked.search);
          } else {
            anchor.removeAttribute("href");
            anchor.classList.add("disabled-link");
            anchor.setAttribute("title", "External link disabled by bounded proxy viewer");
          }
        } catch (_) {
          anchor.removeAttribute("href");
          anchor.classList.add("disabled-link");
        }
      });
    }

    function showFallbackIfEmpty(content, originalHTML) {
      const visibleText = (content.textContent || "").replace(/\s+/g, " ").trim();
      if (visibleText.length >= 30) {
        return;
      }

      const warning = document.createElement("div");
      warning.className = "empty-render";
      warning.textContent = "The target returned HTML, but the sanitized view has little visible text. This page may depend on disabled scripts, external assets, frames, or client-side rendering.";
      shadow.appendChild(warning);

      const details = document.createElement("details");
      const summary = document.createElement("summary");
      summary.textContent = "Show raw HTML preview";
      details.appendChild(summary);

      const preview = document.createElement("pre");
      preview.className = "source-preview";
      preview.textContent = originalHTML.slice(0, 20000);
      details.appendChild(preview);
      shadow.appendChild(details);
    }
  </script>
</body>
</html>
`
