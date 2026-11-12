// Bulk "Assign to Group" works on changes, not on a full group list, so a
// host keeps every group the user did not touch (issue #1245).

// How many of the selected hosts are in each group, by group id.
export const countGroupMembers = (hosts, selectedHostIds) => {
	const selected = new Set(selectedHostIds);
	const counts = {};
	for (const host of hosts || []) {
		if (!selected.has(host.id)) continue;
		for (const membership of host.host_group_memberships || []) {
			const id = membership.host_groups?.id;
			if (id) counts[id] = (counts[id] || 0) + 1;
		}
	}
	return counts;
};

// true when every selected host is in the group, false when none is, and
// "mixed" when only some are. A choice the user made wins.
export const groupCheckState = (groupId, counts, total, choices) => {
	if (groupId in choices) return choices[groupId];
	const count = counts[groupId] || 0;
	if (count === 0) return false;
	return count >= total ? true : "mixed";
};

// Groups to add to and remove from every selected host. Groups the user did
// not change are left out, so the server keeps them as they are on each host.
export const bulkGroupChanges = (counts, total, choices) => {
	const addGroupIds = [];
	const removeGroupIds = [];
	for (const [groupId, checked] of Object.entries(choices)) {
		const count = counts[groupId] || 0;
		if (checked && count < total) addGroupIds.push(groupId);
		if (!checked && count > 0) removeGroupIds.push(groupId);
	}
	return { addGroupIds, removeGroupIds };
};
