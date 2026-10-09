import type { Metadata } from "next";
import "./globals.css";
export const metadata: Metadata = {
  title: "AssetHub · Your assets, together",
  description: "A private home for your applications’ assets.",
};
export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
