import { EvaluateResponse } from "./types";

interface CacheEntry {
  value: EvaluateResponse;
  cachedTs: number;
}

/** TTL + LRU cache for evaluation results. */
export class EvaluationCache {
  private readonly ttlMs: number;
  private readonly maxEntries: number;
  private readonly map = new Map<string, CacheEntry>();

  public constructor(ttlSeconds: number, maxEntries: number) {
    this.ttlMs = ttlSeconds * 1000;
    this.maxEntries = maxEntries;
  }

  public get(key: string, nowMs: number): EvaluateResponse | null {
    const entry = this.map.get(key);
    if (!entry) {
      return null;
    }
    if (nowMs - entry.cachedTs > this.ttlMs) {
      this.map.delete(key);
      return null;
    }
    this.map.delete(key);
    this.map.set(key, entry);
    return { ...entry.value, cacheHit: true, evaluatedTs: nowMs };
  }

  public set(key: string, value: EvaluateResponse, nowMs: number): void {
    if (this.map.has(key)) {
      this.map.delete(key);
    }
    this.map.set(key, { value: { ...value, cacheHit: false }, cachedTs: nowMs });
    if (this.map.size > this.maxEntries) {
      const first = this.map.keys().next();
      if (!first.done) {
        this.map.delete(first.value);
      }
    }
  }

  public clear(): void {
    this.map.clear();
  }

  /** Remove entries whose cached decision used the given bundle name. */
  public invalidateForBundle(bundleName: string): void {
    const toDelete: string[] = [];
    for (const [key, entry] of this.map) {
      if (entry.value.activeBundleNames.includes(bundleName)) {
        toDelete.push(key);
      }
    }
    for (const key of toDelete) {
      this.map.delete(key);
    }
  }

  public size(): number {
    return this.map.size;
  }
}
