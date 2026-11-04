import type { Metadata } from "next";
import type { ReactNode } from "react";

import { Analytics } from "@/components/analytics";

export const metadata: Metadata = {
  title: "Acme",
  description: "Subscription billing without the busywork.",
  metadataBase: new URL(process.env.NEXT_PUBLIC_APP_URL ?? "http://localhost:3000"),
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>
        <Analytics />
        {children}
      </body>
    </html>
  );
}
