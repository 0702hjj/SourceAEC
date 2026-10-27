// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, cleanup, render, waitFor } from "@testing-library/react";

const IDENTITY = [1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1];
const loadOptions = vi.hoisted(() => vi.fn());
const lifecycle = vi.hoisted(() => ({
  autoLoad: true,
  handlers: [] as Record<string, (value?: unknown) => void>[],
  destroy: vi.fn(),
}));

vi.mock("@xeokit/xeokit-sdk", () => ({
  Viewer: class {
    camera = { eye: [0, 0, 1], look: [0, 0, 0], up: [0, 1, 0], viewMatrix: IDENTITY, projMatrix: IDENTITY };
    cameraFlight = { flyTo: vi.fn() };
    cameraControl = { on: vi.fn(() => ({})), off: vi.fn() };
    scene = {
      objects: {},
      selectedObjectIds: [],
      canvas: { canvas: { offsetWidth: 800, offsetHeight: 600 } },
      on: vi.fn(() => ({})),
      off: vi.fn(),
    };
    metaScene = { metaModels: {} };
    destroy = lifecycle.destroy;
  },
  NavCubePlugin: class {},
  XKTLoaderPlugin: class {
    load(options: unknown) {
      loadOptions(options);
      const handlers: Record<string, (value?: unknown) => void> = {};
      lifecycle.handlers.push(handlers);
      return {
        id: "model",
        on: (event: string, cb: (value?: unknown) => void) => {
          handlers[event] = cb;
          if (event === "loaded" && lifecycle.autoLoad) setTimeout(cb, 0);
        },
      };
    }
  },
}));

vi.mock("@/api/client", () => ({
  modelAssetUrl: (id: string, file: string, generation?: string) => `/models/${id}/${file}?generation=${generation ?? ""}`,
}));

import { ViewerProvider } from "./ViewerContext";
import { useViewerStore } from "./store";

afterEach(cleanup);
beforeEach(() => {
  lifecycle.autoLoad = true;
  lifecycle.handlers = [];
  lifecycle.destroy.mockClear();
  loadOptions.mockClear();
});

describe("ViewerProvider", () => {
  it("loads geometry and metadata from the same observed generation", async () => {
    const { rerender } = render(<ViewerProvider modelId="m_1" generation="a" />);
    await waitFor(() => expect(loadOptions).toHaveBeenLastCalledWith(expect.objectContaining({
      src: "/models/m_1/model.xkt?generation=a", metaModelSrc: "/models/m_1/metadata.json?generation=a",
    })));
    rerender(<ViewerProvider modelId="m_1" generation="b" />);
    await waitFor(() => expect(loadOptions).toHaveBeenLastCalledWith(expect.objectContaining({
      src: "/models/m_1/model.xkt?generation=b", metaModelSrc: "/models/m_1/metadata.json?generation=b",
    })));
  });
  it("ignores completion callbacks from a replaced generation", async () => {
    lifecycle.autoLoad = false;
    const { rerender, container } = render(<ViewerProvider modelId="m_1" generation="a" />);
    const old = lifecycle.handlers[0];
    rerender(<ViewerProvider modelId="m_1" generation="b" />);
    expect(lifecycle.destroy).toHaveBeenCalledTimes(1);
    act(() => {
      old.loaded();
      old.error(new Error("obsolete conversion"));
    });
    expect(container.querySelector(".issue-pins-layer")).toBeNull();
    expect(container.textContent).not.toContain("obsolete conversion");
    act(() => lifecycle.handlers[1].loaded());
    await waitFor(() => expect(container.querySelector(".issue-pins-layer")).toBeTruthy());
  });
  it("renders the issue pins layer inside the canvas wrap", async () => {
    useViewerStore.setState({ issues: [], selectedIssueId: null });
    const { container } = render(<ViewerProvider modelId="m_1" />);
    await waitFor(() =>
      expect(container.querySelector(".issue-pins-layer")).toBeTruthy()
    );
    const wrap = container.querySelector(".viewer-canvas-wrap");
    expect(wrap).toBeTruthy();
    expect(wrap!.querySelector(":scope > .issue-pins-layer")).toBeTruthy();
  });
});
