import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";

import { getApiBaseUrl } from "../../../../lib/api";

function coerce(value: FormDataEntryValue) {
  if (typeof value !== "string") {
    return value;
  }
  const trimmed = value.trim();
  if (trimmed === "true") return true;
  if (trimmed === "false") return false;
  if (/^-?\d+$/.test(trimmed)) return Number(trimmed);
  if (/^-?\d+\.\d+$/.test(trimmed)) return Number(trimmed);
  if ((trimmed.startsWith("{") && trimmed.endsWith("}")) || (trimmed.startsWith("[") && trimmed.endsWith("]"))) {
    try {
      return JSON.parse(trimmed);
    } catch {
      return value;
    }
  }
  return value;
}

async function forward(req: NextRequest, { params }: { params: { path: string[] } }) {
  const cookieHeader = cookies()
    .getAll()
    .map(({ name, value }) => `${name}=${value}`)
    .join("; ");

  const targetPath = params.path.join("/");
  let method = req.method;
  let body: BodyInit | undefined;
  const headers = new Headers({ cookie: cookieHeader });
  const contentType = req.headers.get("content-type") || "";
  let redirectTo = req.nextUrl.searchParams.get("redirect_to") || "";

  if (contentType.includes("application/json")) {
    body = await req.text();
    headers.set("content-type", "application/json");
  } else if (contentType.includes("multipart/form-data") || contentType.includes("application/x-www-form-urlencoded")) {
    const formData = await req.formData();
    const payload: Record<string, unknown> = {};
    for (const [key, raw] of formData.entries()) {
      if (key === "_method") {
        method = String(raw).toUpperCase();
        continue;
      }
      if (key === "_redirect") {
        redirectTo = String(raw);
        continue;
      }
      if (key === "_passthrough") {
        continue;
      }
      const value = coerce(raw);
      if (key in payload) {
        const current = payload[key];
        payload[key] = Array.isArray(current) ? [...current, value] : [current, value];
      } else {
        payload[key] = value;
      }
    }
    headers.set("content-type", "application/json");
    body = JSON.stringify(payload);
  }

  const response = await fetch(`${getApiBaseUrl()}/${targetPath}`, {
    method,
    headers,
    body,
    cache: "no-store",
  });

  if (redirectTo && response.ok) {
    return NextResponse.redirect(new URL(redirectTo, req.url));
  }

  const text = await response.text();
  return new NextResponse(text, {
    status: response.status,
    headers: {
      "content-type": response.headers.get("content-type") || "application/json",
    },
  });
}

export async function POST(req: NextRequest, ctx: { params: { path: string[] } }) {
  return forward(req, ctx);
}

export async function PUT(req: NextRequest, ctx: { params: { path: string[] } }) {
  return forward(req, ctx);
}

export async function DELETE(req: NextRequest, ctx: { params: { path: string[] } }) {
  return forward(req, ctx);
}
