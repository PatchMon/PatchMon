import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
	act,
	fireEvent,
	render,
	screen,
	waitFor,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { BulkAssignModal } from "../../pages/Hosts";
import { hostGroupsAPI } from "../../utils/api";

vi.mock("../../utils/api", async (importOriginal) => ({
	...(await importOriginal()),
	hostGroupsAPI: { list: vi.fn(), create: vi.fn() },
}));

const existing = { id: "existing", name: "Existing" };
const created = { id: "new", name: "New group" };
const createResponse = { ID: "new", Name: "New group" };
const setup = (error) => {
	const onAssign = vi.fn();
	const onClose = vi.fn();
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	render(
		<QueryClientProvider client={client}>
			<BulkAssignModal
				selectedHosts={["one", "two"]}
				hosts={[
					{ id: "one", friendly_name: "Host one" },
					{ id: "two", friendly_name: "Host two" },
				]}
				onAssign={onAssign}
				onClose={onClose}
				isLoading={false}
				error={error}
			/>
		</QueryClientProvider>,
	);
	return { onAssign, onClose, client };
};
const enterName = async (name = "New group") => {
	fireEvent.click(screen.getByRole("button", { name: "Create new group" }));
	fireEvent.change(screen.getByLabelText("New group name"), {
		target: { value: name },
	});
};

beforeEach(() => {
	vi.resetAllMocks();
	hostGroupsAPI.list.mockResolvedValue({ data: [existing] });
	hostGroupsAPI.create.mockImplementation(async () => {
		hostGroupsAPI.list.mockResolvedValue({ data: [existing, created] });
		return { data: createResponse };
	});
});

describe("Creating a group during bulk assignment", () => {
	it("shows assignment failures instead of silently leaving the dialog open", () => {
		setup({ response: { data: { error: "Failed to update host groups" } } });
		expect(screen.getByRole("alert")).toHaveTextContent(
			"Failed to update host groups",
		);
	});

	it("keeps the hosts and group selections, selects the new group, and assigns only on confirmation", async () => {
		const { onAssign, client } = setup();
		fireEvent.click(await screen.findByRole("checkbox", { name: "Existing" }));
		await enterName("  New group  ");
		fireEvent.click(screen.getByRole("button", { name: "Create and select" }));
		expect(
			await screen.findByRole("checkbox", { name: "New group" }),
		).toBeChecked();
		expect(screen.getByRole("checkbox", { name: "Existing" })).toBeChecked();
		expect(hostGroupsAPI.create).toHaveBeenCalledWith({ name: "New group" });
		expect(screen.getByText("• Host one")).toBeInTheDocument();
		expect(screen.getByText("• Host two")).toBeInTheDocument();
		expect(client.getQueryData(["hostGroups"])).toContainEqual(created);
		expect(onAssign).not.toHaveBeenCalled();
		fireEvent.click(screen.getByRole("button", { name: "Assign to Groups" }));
		expect(onAssign).toHaveBeenCalledWith(["existing", "new"]);
	});

	it("supports creating the first group and rejects whitespace-only names", async () => {
		hostGroupsAPI.list.mockResolvedValue({ data: [] });
		setup();
		await enterName("   ");
		expect(
			screen.getByRole("button", { name: "Create and select" }),
		).toBeDisabled();
		fireEvent.change(screen.getByLabelText("New group name"), {
			target: { value: "New group" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Create and select" }));
		expect(
			await screen.findByRole("checkbox", { name: "New group" }),
		).toBeChecked();
	});

	it("retains the name and selections after a server error so creation can be retried", async () => {
		hostGroupsAPI.create.mockRejectedValueOnce({
			response: { data: { error: "Permission denied" } },
		});
		const { onAssign } = setup();
		fireEvent.click(await screen.findByRole("checkbox", { name: "Existing" }));
		await enterName();
		fireEvent.click(screen.getByRole("button", { name: "Create and select" }));
		expect(await screen.findByRole("alert")).toHaveTextContent(
			"Permission denied",
		);
		expect(screen.getByLabelText("New group name")).toHaveValue("New group");
		expect(screen.getByRole("checkbox", { name: "Existing" })).toBeChecked();
		expect(onAssign).not.toHaveBeenCalled();
		fireEvent.click(screen.getByRole("button", { name: "Create and select" }));
		expect(
			await screen.findByRole("checkbox", { name: "New group" }),
		).toBeChecked();
	});

	it("blocks assignment and dismissal while creation is pending", async () => {
		let resolve;
		hostGroupsAPI.create.mockReturnValue(
			new Promise((done) => {
				resolve = done;
			}),
		);
		const { onAssign, onClose } = setup();
		await enterName();
		fireEvent.click(screen.getByRole("button", { name: "Create and select" }));
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: "Creating..." }),
			).toBeDisabled(),
		);
		for (const name of [
			"Assign to Groups",
			"Cancel",
			"Cancel new group",
			"Close group assignment",
		]) {
			expect(screen.getByRole("button", { name, exact: true })).toBeDisabled();
		}
		expect(onAssign).not.toHaveBeenCalled();
		expect(onClose).not.toHaveBeenCalled();
		hostGroupsAPI.list.mockResolvedValue({ data: [existing, created] });
		await act(async () => resolve({ data: createResponse }));
		expect(
			await screen.findByRole("checkbox", { name: "New group" }),
		).toBeChecked();
	});

	it("cancels creation without creating a group or losing the assignment selection", async () => {
		const { onAssign } = setup();
		fireEvent.click(await screen.findByRole("checkbox", { name: "Existing" }));
		await enterName();
		fireEvent.click(screen.getByRole("button", { name: "Cancel new group" }));
		expect(hostGroupsAPI.create).not.toHaveBeenCalled();
		fireEvent.click(screen.getByRole("button", { name: "Assign to Groups" }));
		expect(onAssign).toHaveBeenCalledWith(["existing"]);
	});
});
