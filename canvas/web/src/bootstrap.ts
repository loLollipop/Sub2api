import axios, { type InternalAxiosRequestConfig } from "axios";
import { setCanvasActivity } from "./lib/host-activity";

export interface CanvasHostSession {
    type: "sub2api-canvas-session";
    ownerId: number;
    groupId: number;
    groupName: string;
    apiKey: string;
    baseUrl: string;
    theme: "light" | "dark";
    lang: string;
}

declare global {
    interface Window { __SUB2API_CANVAS_SESSION__: CanvasHostSession }
}

async function start(session: CanvasHostSession) {
    window.__SUB2API_CANVAS_SESSION__ = session;
    const pending = new Set<InternalAxiosRequestConfig>();
    const reportBusy = () => setCanvasActivity("request", pending.size);
    const finish = (request?: InternalAxiosRequestConfig) => {
        if (request && pending.delete(request)) reportBusy();
    };
    // Only the host's image gateway receives a credential. This also covers
    // imported node/model settings, rather than trusting a hidden UI control.
    axios.interceptors.request.use((request) => {
        const url = new URL(axios.getUri(request), window.location.origin);
        const method = request.method?.toUpperCase() || "GET";
        const allowed = url.origin === window.location.origin &&
            ((method === "GET" && url.pathname === "/v1/models") ||
             (method === "POST" && /^\/v1\/images\/(generations|edits)$/.test(url.pathname)));
        if (!allowed) throw new Error("此画布仅支持本站生图接口");
        request.headers.set("Authorization", `Bearer ${session.apiKey}`);
        request.withCredentials = false;
        if (method === "POST") { pending.add(request); reportBusy(); }
        return request;
    });
    axios.interceptors.response.use(response => {
        finish(response.config);
        return response;
    }, (error: unknown) => {
        if (axios.isAxiosError(error)) finish(error.config);
        return Promise.reject(error);
    });
    const response = await fetch("/v1/models", {
        credentials: "omit", cache: "no-store", redirect: "error",
        headers: { Authorization: `Bearer ${session.apiKey}` },
    });
    if (!response.ok) throw new Error(`读取生图模型失败（HTTP ${response.status}）`);
    const payload = await response.json() as { data?: { id: string }[] };
    const advertised = (payload.data || []).map(model => model.id).filter(id => typeof id === "string" && !/video|audio|sora|veo|whisper|tts/i.test(id));
    const images = advertised.filter(id => /image|dall-e|flux|imagen|imagine(?!.*video)|banana/i.test(id));
    const names = images.length ? images : advertised;
    if (!names.length) throw new Error("当前生图分组没有可用模型");

    // Store modules are imported only after ownerId is known, so their existing
    // localforage databases and Zustand keys are scoped to this signed-in user.
    const { useConfigStore, encodeChannelModel } = await import("./stores/use-config-store");
    const channelId = `sub2api-${session.groupId}`;
    const models = names.map(name => encodeChannelModel(channelId, name));
    useConfigStore.setState(state => ({ config: {
        ...state.config, channelMode: "local", apiFormat: "openai",
        apiKey: session.apiKey, baseUrl: window.location.origin,
        channels: [{ id: channelId, name: session.groupName, baseUrl: window.location.origin,
            apiKey: session.apiKey, apiFormat: "openai", models: names.map(name => ({ name, capability: "image" })) }],
        models, model: models[0], imageModel: models[0], videoModel: "", audioModel: "", textModel: "",
        proxyEnabled: false, proxyUrl: "", canvasImageCount: state.config.canvasImageCount || "1",
    } }));
    const { useThemeStore } = await import("./stores/use-theme-store");
    useThemeStore.getState().setTheme(session.theme);
    const { default: i18n } = await import("./i18n");
    await i18n.changeLanguage(session.lang.toLowerCase().startsWith("zh") ? "zh-CN" : "en-US");
    await import("./main");
}

let started = false;
window.addEventListener("message", event => {
    if (started || event.origin !== window.location.origin || event.source !== window.parent) return;
    const session = event.data as Partial<CanvasHostSession> | null;
    if (session?.type !== "sub2api-canvas-session" || !Number.isInteger(session.ownerId) ||
        !Number.isInteger(session.groupId) || !session.apiKey || session.baseUrl !== window.location.origin) return;
    started = true;
    void start(session as CanvasHostSession).catch(error => {
        const root = document.getElementById("root");
        if (root) root.textContent = error instanceof Error ? error.message : "无法加载画布，请刷新重试";
    });
});
if (window.parent === window) {
    const root = document.getElementById("root");
    if (root) root.textContent = "请从控制台的「无限画布」入口打开";
} else {
    window.parent.postMessage({ type: "sub2api-canvas-ready" }, window.location.origin);
}
