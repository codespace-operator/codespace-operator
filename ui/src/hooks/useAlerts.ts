import { useState } from "react";

export function useAlerts(max = 5) {
  const [alerts, setAlerts] = useState<
    { key: string; title: string; variant: any }[]
  >([]);
  return {
    push: (title: string, variant: any = "info") =>
      setAlerts((a) =>
        [
          { key: Math.random().toString(36).slice(2), title, variant },
          ...a,
        ].slice(0, max),
      ),
    close: (key: string) => setAlerts((s) => s.filter((x) => x.key !== key)),
    list: alerts,
  };
}
