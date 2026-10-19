import { Navigate } from "react-router";
import { useSession } from "@/shared/lib/session";
import { NoPermission } from "@/shared/ui/patterns";
import { SETTINGS_SECTIONS } from "../components/sections";

/** /settings → first section the user may open. */
export default function SettingsIndexPage() {
  const { can } = useSession();
  const first = SETTINGS_SECTIONS.find((s) => can(s.permission));
  return first ? <Navigate to={first.to} replace /> : <NoPermission />;
}
