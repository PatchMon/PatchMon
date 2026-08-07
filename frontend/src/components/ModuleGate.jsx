import { useAuth } from "../contexts/AuthContext";
import UpgradeRequired from "../pages/UpgradeRequired";

// ModuleGate checks tenant.modules (via AuthContext.hasModule) and either
// renders children (if the module is enabled) or the UpgradeRequired screen.
// Used in App.jsx around every tier-gated route. The URL stays the same so
// bookmarks keep working after upgrade.
//
// This is purely cosmetic — the real access control lives in the server's
// hostctx.RequireModule middleware, which returns 403 for any locked API
// call. Bypassing this component in the browser grants no additional access.
const ModuleGate = ({ module: moduleKey, children }) => {
	const { hasModule } = useAuth();

	if (hasModule(moduleKey)) {
		return children;
	}

	return <UpgradeRequired module={moduleKey} />;
};

export default ModuleGate;
