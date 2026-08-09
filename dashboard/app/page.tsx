import { redirect } from "next/navigation";

import PublicLanding from "../components/public-landing";
import { getApiBaseUrl, getAppBaseUrl, getViewerOptional } from "../lib/api";

export default async function LoginPage() {
  let viewer = null;
  try {
    viewer = await getViewerOptional();
  } catch {
    viewer = null;
  }

  if (viewer) {
    redirect("/overview");
  }

  return (
    <PublicLanding
      action={`${getApiBaseUrl()}/v1/dashboard/login`}
      redirectTo={`${getAppBaseUrl()}/overview`}
    />
  );
}
