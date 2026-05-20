import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { NextRequest, NextResponse } from "next/server";

type RecordedWebhook = {
  method: string;
  headers: Record<string, string>;
  body: any;
  rawBody: string;
  created_at: string;
};

const rootDir = path.join(os.tmpdir(), "paygate-test-webhooks");

function tokenFrom(request: NextRequest) {
  const token = request.nextUrl.searchParams.get("token")?.trim() || "default";
  return token.replace(/[^a-zA-Z0-9_-]/g, "_");
}

function bucketPath(token: string) {
  return path.join(rootDir, `${token}.json`);
}

async function loadBucket(token: string): Promise<RecordedWebhook[]> {
  try {
    const raw = await fs.readFile(bucketPath(token), "utf8");
    return JSON.parse(raw) as RecordedWebhook[];
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return [];
    }
    throw error;
  }
}

async function saveBucket(token: string, items: RecordedWebhook[]) {
  await fs.mkdir(rootDir, { recursive: true });
  await fs.writeFile(bucketPath(token), JSON.stringify(items, null, 2));
}

export async function GET(request: NextRequest) {
  const token = tokenFrom(request);
  const items = await loadBucket(token);
  return NextResponse.json({
    entity: "collection",
    count: items.length,
    items,
  });
}

export async function DELETE(request: NextRequest) {
  const token = tokenFrom(request);
  await fs.rm(bucketPath(token), { force: true });
  return NextResponse.json({ status: "cleared" });
}

export async function POST(request: NextRequest) {
  const token = tokenFrom(request);
  const rawBody = await request.text();
  let body: any = rawBody;
  try {
    body = rawBody ? JSON.parse(rawBody) : null;
  } catch {
    body = rawBody;
  }

  const headers: Record<string, string> = {};
  request.headers.forEach((value, key) => {
    headers[key] = value;
  });

  const entry: RecordedWebhook = {
    method: request.method,
    headers,
    body,
    rawBody,
    created_at: new Date().toISOString(),
  };

  const items = await loadBucket(token);
  items.push(entry);
  await saveBucket(token, items);

  return NextResponse.json({ status: "ok" });
}
