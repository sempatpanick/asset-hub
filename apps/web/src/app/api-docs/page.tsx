import type { Metadata } from "next";
import { ApiDocs } from "@/features/api-docs/api-docs";
import "./styles.css";

export const metadata: Metadata = {
  title: "API Docs | AssetHub",
  description:
    "AssetHub API integration guide, complete endpoint reference and interactive request console.",
};
export default function ApiDocsPage() {
  return <ApiDocs />;
}
