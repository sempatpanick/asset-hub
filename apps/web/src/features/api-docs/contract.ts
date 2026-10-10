import specification from "../../../public/api-docs/openapi.json";

export type Schema = {
  $ref?: string;
  type?: string | string[];
  format?: string;
  description?: string;
  required?: string[];
  properties?: Record<string, Schema>;
  items?: Schema;
  allOf?: Schema[];
  enum?: unknown[];
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  minItems?: number;
  pattern?: string;
  default?: unknown;
};
export type Parameter = {
  $ref?: string;
  in: string;
  name: string;
  required?: boolean;
  description?: string;
  schema: Schema;
  example?: unknown;
};
export type Media = { schema: Schema; example?: unknown };
export type Response = {
  description: string;
  content?: Record<string, Media>;
  headers?: Record<
    string,
    { description?: string; schema: Schema; example?: unknown }
  >;
  "x-response-example"?: string;
};
export type Operation = {
  operationId: string;
  summary: string;
  description: string;
  tags: string[];
  "x-access": string;
  security?: Record<string, string[]>[];
  servers?: { url: string }[];
  parameters?: Parameter[];
  requestBody?: {
    $ref?: string;
    required?: boolean;
    content: Record<string, Media>;
  };
  responses: Record<string, Response>;
};
type Contract = {
  info: { title: string; version: string };
  paths: Record<string, { parameters?: Parameter[] } & Record<string, unknown>>;
  components: Record<string, Record<string, unknown>>;
};
export const contract = specification as unknown as Contract;
export function resolve<T extends { $ref?: string }>(value: T): T {
  if (!value.$ref) return value;
  const [, , kind, name] = value.$ref.split("/");
  return contract.components[kind][name] as T;
}
export const endpoints = Object.entries(contract.paths).flatMap(
  ([path, item]) =>
    ["get", "post", "put", "patch", "delete", "head"].flatMap((method) => {
      const operation = item[method] as Operation | undefined;
      return operation
        ? [
            {
              ...operation,
              path,
              method: method.toUpperCase(),
              parameters: [
                ...(item.parameters ?? []),
                ...(operation.parameters ?? []),
              ].map(resolve),
            },
          ]
        : [];
    }),
);
export type Endpoint = (typeof endpoints)[number];
export function fields(schema: Schema): Record<string, Schema> {
  const value = resolve(schema);
  return Object.assign(
    {},
    ...(value.allOf ?? []).map(fields),
    value.properties ?? {},
  );
}
export function requiredFields(schema: Schema): string[] {
  const value = resolve(schema);
  return [
    ...(value.required ?? []),
    ...(value.allOf ?? []).flatMap(requiredFields),
  ];
}
export function schemaLabel(schema: Schema): string {
  const value = resolve(schema);
  const rules = [
    Array.isArray(value.type)
      ? value.type.join(" or ")
      : (value.type ?? (value.allOf ? "object" : "any")),
    value.format,
    value.enum ? `allowed: ${value.enum.join(", ")}` : "",
    value.minimum !== undefined ? `min ${value.minimum}` : "",
    value.maximum !== undefined ? `max ${value.maximum}` : "",
    value.minLength !== undefined ? `min length ${value.minLength}` : "",
    value.maxLength !== undefined ? `max length ${value.maxLength}` : "",
    value.minItems !== undefined ? `min items ${value.minItems}` : "",
    value.pattern ? `pattern ${value.pattern}` : "",
    value.default !== undefined ? `default ${value.default}` : "",
  ];
  return rules.filter(Boolean).join(" · ");
}
