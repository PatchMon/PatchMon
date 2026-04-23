import UpgradeRequiredContent from "../components/UpgradeRequiredContent";

// Full-page upgrade screen rendered by <ModuleGate> when the current plan
// doesn't include the required module. Keeps the route URL intact so that
// bookmarks continue to work after an upgrade.
const UpgradeRequired = ({ module: moduleKey }) => {
	return <UpgradeRequiredContent module={moduleKey} variant="page" />;
};

export default UpgradeRequired;
