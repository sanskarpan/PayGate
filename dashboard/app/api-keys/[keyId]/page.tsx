import { notFound } from "next/navigation";

import { getAPIKeys, requireViewer } from "../../../lib/api";
import APIKeyDetailManager from "../../../components/api-key-detail-manager";
import { getApiBaseUrl } from "../../../lib/api";

interface Props {
  params: { keyId: string };
}

export default async function APIKeyAllowlistPage({ params }: Props) {
  await requireViewer();
  const keys = await getAPIKeys();
  const key = keys.items.find((item) => item.id === params.keyId);
  if (!key) {
    notFound();
  }
  return <APIKeyDetailManager apiBaseUrl={getApiBaseUrl()} item={key} />;
}
