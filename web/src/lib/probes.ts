import type { ProbeMetadata } from "@/api/client";

export function probeName(id: number, metadata?: ProbeMetadata): string {
  return metadata?.display_name?.trim() || `Probe ${id}`;
}

export function probeSubtitle(id: number, metadata?: ProbeMetadata): string {
  const parts = [`Probe ${id}`];
  if (metadata?.probe_type) parts.push(metadata.probe_type);
  if (metadata?.country_code) parts.push(metadata.country_code);
  return parts.join(" · ");
}
