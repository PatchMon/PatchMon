import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { useQuery } from "@tanstack/react-query";
import { Maximize2, Pause, Play, Search, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { sshRecordingsAPI } from "../utils/api";

const RecordingPlayer = ({ session, onClose }) => {
	const containerRef = useRef(null);
	const terminalRef = useRef(null);
	const fitRef = useRef(null);
	const frameRef = useRef(null);
	const startedRef = useRef(0);
	const baseOffsetRef = useRef(0);
	const [playing, setPlaying] = useState(false);
	const [speed, setSpeed] = useState(1);
	const [position, setPosition] = useState(0);
	const positionRef = useRef(0);
	const renderedEventIndexRef = useRef(0);
	const [search, setSearch] = useState("");
	const { data, isLoading } = useQuery({
		queryKey: ["ssh-recording-events", session.id],
		queryFn: () =>
			sshRecordingsAPI
				.events(session.id)
				.then((response) => response.data.events || []),
	});
	const events = data || [];
	const duration = events.length ? events[events.length - 1].t : 0;

	useEffect(() => {
		const previousOverflow = document.body.style.overflow;
		document.body.style.overflow = "hidden";
		return () => {
			document.body.style.overflow = previousOverflow;
		};
	}, []);

	useEffect(() => {
		const terminal = new Terminal({
			convertEol: false,
			cursorBlink: false,
			scrollback: 10000,
			theme: { background: "#000000" },
		});
		const fit = new FitAddon();
		terminal.loadAddon(fit);
		terminal.open(containerRef.current);
		fit.fit();
		terminalRef.current = terminal;
		fitRef.current = fit;
		const resize = () => fit.fit();
		window.addEventListener("resize", resize);
		return () => {
			window.removeEventListener("resize", resize);
			terminal.dispose();
			if (frameRef.current) cancelAnimationFrame(frameRef.current);
		};
	}, []);

	const renderAt = useCallback(
		(target) => {
			const terminal = terminalRef.current;
			if (!terminal) return;
			terminal.reset();
			let renderedEventIndex = 0;
			for (const event of events) {
				if (event.t > target) break;
				if (event.type === "output") terminal.write(event.data || "");
				if (event.type === "resize" && event.cols && event.rows)
					terminal.resize(event.cols, event.rows);
				renderedEventIndex += 1;
			}
			renderedEventIndexRef.current = renderedEventIndex;
			positionRef.current = target;
			setPosition(target);
		},
		[events],
	);

	const advanceTo = useCallback(
		(target) => {
			const terminal = terminalRef.current;
			if (!terminal) return;
			let index = renderedEventIndexRef.current;
			while (index < events.length && events[index].t <= target) {
				const event = events[index];
				if (event.type === "output") terminal.write(event.data || "");
				if (event.type === "resize" && event.cols && event.rows)
					terminal.resize(event.cols, event.rows);
				index += 1;
			}
			renderedEventIndexRef.current = index;
			positionRef.current = target;
			setPosition(target);
		},
		[events],
	);

	useEffect(() => {
		if (events.length) renderAt(0);
	}, [events, renderAt]);
	useEffect(() => {
		if (!playing) {
			if (frameRef.current) cancelAnimationFrame(frameRef.current);
			return;
		}
		startedRef.current = performance.now();
		baseOffsetRef.current = positionRef.current;
		const tick = (now) => {
			const next = Math.min(
				duration,
				baseOffsetRef.current + (now - startedRef.current) * 1000 * speed,
			);
			advanceTo(next);
			if (next >= duration) {
				setPlaying(false);
				return;
			}
			frameRef.current = requestAnimationFrame(tick);
		};
		frameRef.current = requestAnimationFrame(tick);
		return () => frameRef.current && cancelAnimationFrame(frameRef.current);
	}, [playing, speed, duration, advanceTo]);

	const searchMatches = useMemo(() => {
		if (!search.trim()) return [];
		const needle = search.toLowerCase();
		return events.filter(
			(event) =>
				event.type === "output" &&
				(event.data || "").toLowerCase().includes(needle),
		);
	}, [events, search]);

	return createPortal(
		<div className="fixed inset-0 z-[100] bg-black/80 p-4 flex flex-col">
			<div className="bg-secondary-900 border border-secondary-700 rounded-lg flex flex-col flex-1 min-h-0">
				<div className="p-3 border-b border-secondary-700 flex items-center gap-3">
					<button
						type="button"
						onClick={() => setPlaying(!playing)}
						className="p-2 bg-primary-600 rounded"
					>
						{playing ? <Pause size={16} /> : <Play size={16} />}
					</button>
					<select
						value={speed}
						onChange={(event) => setSpeed(Number(event.target.value))}
						className="bg-secondary-800 rounded px-2 py-1"
					>
						<option value={0.5}>0.5x</option>
						<option value={1}>1x</option>
						<option value={2}>2x</option>
						<option value={4}>4x</option>
					</select>
					<input
						type="range"
						min="0"
						max={duration || 1}
						value={position}
						onChange={(event) => {
							setPlaying(false);
							renderAt(Number(event.target.value));
						}}
						className="flex-1"
					/>
					<span className="text-xs text-secondary-300">
						{(position / 1000000).toFixed(1)}s /{" "}
						{(duration / 1000000).toFixed(1)}s
					</span>
					<button
						type="button"
						onClick={() => containerRef.current?.requestFullscreen()}
						title="Fullscreen"
					>
						<Maximize2 size={18} />
					</button>
					<button type="button" onClick={onClose}>
						<X size={20} />
					</button>
				</div>
				<div className="p-2 border-b border-secondary-700 flex gap-2 items-center">
					<Search size={16} />
					<input
						value={search}
						onChange={(event) => setSearch(event.target.value)}
						placeholder="Search terminal output"
						className="bg-secondary-800 px-2 py-1 rounded flex-1"
					/>
					<span className="text-xs text-secondary-400">
						{searchMatches.length} matches
					</span>
					{searchMatches[0] && (
						<button
							type="button"
							onClick={() => renderAt(searchMatches[0].t)}
							className="text-xs text-primary-300"
						>
							Go to first
						</button>
					)}
				</div>
				<div className="flex-1 min-h-0 relative">
					<div ref={containerRef} className="absolute inset-0 bg-black p-2" />
					{isLoading && (
						<div className="absolute inset-0 z-10 p-8 bg-secondary-900">
							Loading recording...
						</div>
					)}
				</div>
			</div>
		</div>,
		document.body,
	);
};

const SSHRecordings = () => {
	const [filters, setFilters] = useState({
		hostId: "",
		userId: "",
		linuxUsername: "",
		status: "",
		startedAfter: "",
		startedBefore: "",
	});
	const [selected, setSelected] = useState(null);
	const params = Object.fromEntries(
		Object.entries(filters).filter(([, value]) => value),
	);
	const {
		data = [],
		isLoading,
		error,
	} = useQuery({
		queryKey: ["ssh-recordings", params],
		queryFn: () =>
			sshRecordingsAPI.list(params).then((response) => response.data),
	});
	const change = (key) => (event) =>
		setFilters((current) => ({ ...current, [key]: event.target.value }));
	return (
		<div className="p-6 space-y-5">
			<div>
				<h1 className="text-2xl font-semibold">SSH Sessions</h1>
				<p className="text-secondary-400">
					Sessions opened with patchmon ssh are recorded and replayable; raw
					patchmon tunnel connections are not listed or inspected.
				</p>
			</div>
			<div className="grid grid-cols-2 lg:grid-cols-6 gap-2 bg-secondary-900 p-3 rounded-lg">
				<input
					value={filters.hostId}
					onChange={change("hostId")}
					placeholder="Host ID"
					className="bg-secondary-800 p-2 rounded"
				/>
				<input
					value={filters.userId}
					onChange={change("userId")}
					placeholder="PatchMon user ID"
					className="bg-secondary-800 p-2 rounded"
				/>
				<input
					value={filters.linuxUsername}
					onChange={change("linuxUsername")}
					placeholder="Linux account"
					className="bg-secondary-800 p-2 rounded"
				/>
				<select
					value={filters.status}
					onChange={change("status")}
					className="bg-secondary-800 p-2 rounded"
				>
					<option value="">All statuses</option>
					<option value="completed">Completed</option>
					<option value="failed">Failed</option>
					<option value="disconnected">Disconnected</option>
					<option value="active">Active</option>
				</select>
				<input
					type="datetime-local"
					value={filters.startedAfter.slice(0, 16)}
					onChange={(event) =>
						setFilters((current) => ({
							...current,
							startedAfter: event.target.value
								? new Date(event.target.value).toISOString()
								: "",
						}))
					}
					className="bg-secondary-800 p-2 rounded"
				/>
				<input
					type="datetime-local"
					value={filters.startedBefore.slice(0, 16)}
					onChange={(event) =>
						setFilters((current) => ({
							...current,
							startedBefore: event.target.value
								? new Date(event.target.value).toISOString()
								: "",
						}))
					}
					className="bg-secondary-800 p-2 rounded"
				/>
			</div>
			{error && <div className="text-red-300">Unable to load recordings.</div>}
			<div className="overflow-auto border border-secondary-700 rounded-lg">
				<table className="w-full text-sm">
					<thead className="bg-secondary-800">
						<tr>
							<th className="p-3 text-left">Started</th>
							<th className="p-3 text-left">Host</th>
							<th className="p-3 text-left">PatchMon user</th>
							<th className="p-3 text-left">Linux account</th>
							<th className="p-3 text-left">Status</th>
							<th className="p-3 text-left">Duration</th>
							<th className="p-3"></th>
						</tr>
					</thead>
					<tbody>
						{isLoading ? (
							<tr>
								<td colSpan="7" className="p-6">
									Loading...
								</td>
							</tr>
						) : (
							data.map((session) => (
								<tr key={session.id} className="border-t border-secondary-800">
									<td className="p-3">
										{session.started_at?.Time
											? new Date(session.started_at.Time).toLocaleString()
											: "-"}
									</td>
									<td className="p-3">{session.host_name}</td>
									<td className="p-3">{session.patchmon_username}</td>
									<td className="p-3">{session.linux_username}</td>
									<td className="p-3">{session.status}</td>
									<td className="p-3">
										{session.duration_ms
											? `${(session.duration_ms / 1000).toFixed(1)}s`
											: "-"}
									</td>
									<td className="p-3">
										<button
											type="button"
											disabled={Boolean(session.recording_deleted_at?.Valid)}
											onClick={() => setSelected(session)}
											className="text-primary-300 disabled:opacity-40"
										>
											Replay
										</button>
									</td>
								</tr>
							))
						)}
					</tbody>
				</table>
			</div>
			{selected && (
				<RecordingPlayer session={selected} onClose={() => setSelected(null)} />
			)}
		</div>
	);
};

export default SSHRecordings;
