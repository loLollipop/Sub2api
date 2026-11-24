import localforage from "localforage";
import type { StateStorage } from "zustand/middleware";

const fallbackKey = (name: string) => `sub2api:${window.__SUB2API_CANVAS_SESSION__.ownerId}:${name}`;

localforage.config({
    name: `sub2api-canvas-${window.__SUB2API_CANVAS_SESSION__.ownerId}`,
    storeName: "app_state",
});

export const localForageStorage: StateStorage = {
    getItem: async (name) => {
        if (typeof window === "undefined") return null;
        try {
            return (await localforage.getItem<string>(name)) || null;
        } catch {
            return window.localStorage.getItem(fallbackKey(name));
        }
    },
    setItem: async (name, value) => {
        if (typeof window === "undefined") return;
        try {
            await localforage.setItem(name, value);
        } catch {
            window.localStorage.setItem(fallbackKey(name), value);
        }
    },
    removeItem: async (name) => {
        if (typeof window === "undefined") return;
        try {
            await localforage.removeItem(name);
        } catch {
            window.localStorage.removeItem(fallbackKey(name));
        }
    },
};
