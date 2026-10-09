import type { components } from "./schema";
export type User = components["schemas"]["User"];
export type Project = components["schemas"]["Project"];
export type Asset = components["schemas"]["Asset"];
export type Upload = components["schemas"]["Upload"];
export type ApiKey = components["schemas"]["ApiKey"];
export type Activity = components["schemas"]["Activity"];
export type Overview = components["schemas"]["Overview"];
export type Health = components["schemas"]["Health"];
export type Settings = components["schemas"]["Settings"];
export type Member = components["schemas"]["Member"];
export interface Items<T> {
  items: T[];
  limit?: number;
  offset?: number;
}
