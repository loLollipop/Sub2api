const active = { request: 0, generation: 0 };
let busy = false;

// A response is not finished until the original canvas has saved its image.
export function setCanvasActivity(source: keyof typeof active, count: number) {
    active[source] = count;
    const next = active.request > 0 || active.generation > 0;
    if (next === busy) return;
    busy = next;
    window.parent.postMessage({ type: "sub2api-canvas-busy", busy }, window.location.origin);
}
