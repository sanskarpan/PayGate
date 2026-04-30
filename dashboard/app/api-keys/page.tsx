import APIKeyManager from "../../components/api-key-manager";
import { getAPIKeys, getApiBaseUrl, requireViewer } from "../../lib/api";

export default async function APIKeysPage() {
  await requireViewer();
  const keys = await getAPIKeys();

  return (
    <APIKeyManager apiBaseUrl={getApiBaseUrl()} initialItems={keys.items} />
  );
}
