import { useCallback, useEffect, useRef, useState } from "react";
import { currentScan, startScan, watchScan } from "./api";
import type { Scan } from "./types";

export interface Toast {
  id: number;
  title: string;
  detail: string;
}

/**
 * Scan state for the page: starts scans, follows their progress, raises a
 * toast for every plugin that fails, and calls onFinished when a scan ends.
 * Picks up a scan already running when the page loads.
 */
export function useScan(onFinished: () => void) {
  const [scan, setScan] = useState<Scan | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [starting, setStarting] = useState(false);

  const watching = useRef<AbortController | null>(null);
  const failedSeen = useRef(new Set<string>());
  const nextToastId = useRef(1);
  const onFinishedRef = useRef(onFinished);
  onFinishedRef.current = onFinished;

  const toast = useCallback((title: string, detail: string) => {
    setToasts((ts) => [...ts, { id: nextToastId.current++, title, detail }]);
  }, []);

  const dismissToast = useCallback((id: number) => {
    setToasts((ts) => ts.filter((t) => t.id !== id));
  }, []);

  const apply = useCallback(
    (s: Scan) => {
      setScan(s);
      for (const p of s.plugins) {
        const key = `${s.id}:${p.name}`;
        if (p.status === "FAILED" && !failedSeen.current.has(key)) {
          failedSeen.current.add(key);
          toast(`Scan failed: ${p.name}`, p.error ?? "Unknown error");
        }
      }
    },
    [toast],
  );

  const follow = useCallback(
    (s: Scan) => {
      apply(s);
      if (!s.running) {
        onFinishedRef.current();
        return;
      }
      watching.current?.abort();
      const ctrl = new AbortController();
      watching.current = ctrl;
      let last = s;
      watchScan(s.id, (u) => {
        last = u;
        apply(u);
      }, ctrl.signal)
        .then(() => {
          if (!last.running) onFinishedRef.current();
        })
        .catch((err) => {
          if (ctrl.signal.aborted) return;
          toast("Lost connection to scan progress", err instanceof Error ? err.message : String(err));
        });
    },
    [apply, toast],
  );

  // Reattach to a scan that was already running when the page loaded.
  useEffect(() => {
    currentScan()
      .then((s) => s && follow(s))
      .catch(() => {});
    return () => watching.current?.abort();
  }, [follow]);

  const start = useCallback(async () => {
    setModalOpen(true);
    if (scan?.running) return;
    setStarting(true);
    try {
      follow(await startScan());
    } catch (err) {
      setModalOpen(false);
      toast("Couldn't start scan", err instanceof Error ? err.message : String(err));
    } finally {
      setStarting(false);
    }
  }, [scan, follow, toast]);

  return {
    scan,
    starting,
    start,
    modalOpen,
    openModal: () => setModalOpen(true),
    closeModal: () => setModalOpen(false),
    toasts,
    dismissToast,
  };
}
