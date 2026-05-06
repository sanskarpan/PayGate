import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";

import { getApiBaseUrl } from "../../../../lib/api";

export async function DELETE(
  req: NextRequest,
  { params }: { params: { id: string } },
) {
  const { id } = params;

  const cookieHeader = cookies()
    .getAll()
    .map(({ name, value }) => `${name}=${value}`)
    .join("; ");

  const res = await fetch(`${getApiBaseUrl()}/v1/merchants/me/invitations/${id}`, {
    method: "DELETE",
    headers: { cookie: cookieHeader },
    cache: "no-store",
  });

  if (!res.ok) {
    return NextResponse.json({ error: "revoke failed" }, { status: res.status });
  }

  return NextResponse.redirect(new URL("/team", req.url));
}

// The team page's revoke form uses method="POST" (HTML forms only support GET/POST).
// Handle POST as a method-override for DELETE.
export async function POST(
  req: NextRequest,
  { params }: { params: { id: string } },
) {
  return DELETE(req, { params });
}
