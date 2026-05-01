import { requireViewer } from "../../../lib/api";
import IPAllowlistManager from "../../../components/ip-allowlist-manager";
import { getApiBaseUrl } from "../../../lib/api";

interface Props {
  params: { keyId: string };
}

export default async function APIKeyAllowlistPage({ params }: Props) {
  await requireViewer();
  return (
    <IPAllowlistManager
      keyId={params.keyId}
      apiBaseUrl={getApiBaseUrl()}
    />
  );
}
