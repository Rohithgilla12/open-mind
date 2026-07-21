// React surface over the durable capture queue: pending count for badges,
// flush helper, and AppState / NetInfo / focus-driven sync.
import NetInfo from "@react-native-community/netinfo";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { AppState, type AppStateStatus } from "react-native";
import type { AssetUpload } from "./api";
import {
  enqueue as enqueueRaw,
  enqueueAsset as enqueueAssetRaw,
  flushQueue,
  listQueued,
  subscribeQueue,
  type QueuedCapture,
} from "./capture-queue";
import { drainSharedPending } from "./shared-pending";

type CaptureQueueContextValue = {
  pending: QueuedCapture[];
  pendingCount: number;
  flushing: boolean;
  refresh: () => Promise<void>;
  enqueue: (input: { url?: string; note?: string }) => Promise<{ id: string; deduped: boolean }>;
  enqueueAsset: (files: AssetUpload[]) => Promise<{ ids: string[] }>;
  flush: () => Promise<{ sent: number; remaining: number }>;
};

const CaptureQueueContext = createContext<CaptureQueueContextValue | null>(null);

export function CaptureQueueProvider({ children }: { children: React.ReactNode }) {
  const [pending, setPending] = useState<QueuedCapture[]>([]);
  const [flushing, setFlushing] = useState(false);
  const wasConnected = useRef<boolean | null>(null);

  const refresh = useCallback(async () => {
    setPending(await listQueued());
  }, []);

  const flush = useCallback(async () => {
    setFlushing(true);
    try {
      return await flushQueue();
    } finally {
      setFlushing(false);
      setPending(await listQueued());
    }
  }, []);

  const enqueue = useCallback(async (input: { url?: string; note?: string }) => {
    const result = await enqueueRaw(input);
    setPending(await listQueued());
    return result;
  }, []);

  const enqueueAsset = useCallback(async (files: AssetUpload[]) => {
    const result = await enqueueAssetRaw(files);
    setPending(await listQueued());
    return result;
  }, []);

  const drainAndFlush = useCallback(async () => {
    try {
      await drainSharedPending();
    } catch {
      // Draining is best-effort; a failure must never block the flush.
    }
    return flush();
  }, [flush]);

  useEffect(() => {
    void refresh();
    return subscribeQueue(setPending);
  }, [refresh]);

  // Flush when the app returns to the foreground.
  useEffect(() => {
    const onChange = (next: AppStateStatus) => {
      if (next === "active") void drainAndFlush();
    };
    const sub = AppState.addEventListener("change", onChange);
    // Initial mount flush.
    void drainAndFlush();
    return () => sub.remove();
  }, [drainAndFlush]);

  // Flush when connectivity returns.
  useEffect(() => {
    const unsub = NetInfo.addEventListener((state) => {
      const connected = !!(state.isConnected && state.isInternetReachable !== false);
      if (wasConnected.current === false && connected) {
        void flush();
      }
      wasConnected.current = connected;
    });
    return () => unsub();
  }, [flush]);

  const value = useMemo<CaptureQueueContextValue>(
    () => ({
      pending,
      pendingCount: pending.length,
      flushing,
      refresh,
      enqueue,
      enqueueAsset,
      flush,
    }),
    [pending, flushing, refresh, enqueue, enqueueAsset, flush],
  );

  return (
    <CaptureQueueContext.Provider value={value}>{children}</CaptureQueueContext.Provider>
  );
}

export function useCaptureQueue(): CaptureQueueContextValue {
  const ctx = useContext(CaptureQueueContext);
  if (!ctx) throw new Error("useCaptureQueue must be used within a CaptureQueueProvider");
  return ctx;
}
