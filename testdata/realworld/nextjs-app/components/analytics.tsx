"use client";

import { useEffect } from "react";

export function Analytics() {
  useEffect(() => {
    const key = process.env.NEXT_PUBLIC_POSTHOG_KEY;
    if (!key || process.env.NODE_ENV !== "production") return;

    void import("posthog-js").then(({ default: posthog }) => {
      posthog.init(key, { api_host: "https://eu.i.posthog.com" });
    });
  }, []);

  return null;
}
