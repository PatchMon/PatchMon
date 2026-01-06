import { createContext, useContext, useState, useCallback } from "react";
import { CheckCircle, XCircle, AlertTriangle, Info, X } from "lucide-react";

const ToastContext = createContext(null);

export function ToastProvider({ children }) {
	const [toasts, setToasts] = useState([]);

	const addToast = useCallback((message, type = "info", duration = 5000) => {
		const id = Date.now() + Math.random();
		setToasts((prev) => [...prev, { id, message, type }]);

		if (duration > 0) {
			setTimeout(() => {
				setToasts((prev) => prev.filter((t) => t.id !== id));
			}, duration);
		}

		return id;
	}, []);

	const removeToast = useCallback((id) => {
		setToasts((prev) => prev.filter((t) => t.id !== id));
	}, []);

	const success = useCallback((message, duration) => addToast(message, "success", duration), [addToast]);
	const error = useCallback((message, duration) => addToast(message, "error", duration), [addToast]);
	const warning = useCallback((message, duration) => addToast(message, "warning", duration), [addToast]);
	const info = useCallback((message, duration) => addToast(message, "info", duration), [addToast]);

	return (
		<ToastContext.Provider value={{ addToast, removeToast, success, error, warning, info }}>
			{children}
			<ToastContainer toasts={toasts} onRemove={removeToast} />
		</ToastContext.Provider>
	);
}

function ToastContainer({ toasts, onRemove }) {
	if (toasts.length === 0) return null;

	return (
		<div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 max-w-sm">
			{toasts.map((toast) => (
				<Toast key={toast.id} toast={toast} onRemove={onRemove} />
			))}
		</div>
	);
}

function Toast({ toast, onRemove }) {
	const icons = {
		success: <CheckCircle className="h-5 w-5 text-green-400" />,
		error: <XCircle className="h-5 w-5 text-red-400" />,
		warning: <AlertTriangle className="h-5 w-5 text-yellow-400" />,
		info: <Info className="h-5 w-5 text-blue-400" />,
	};

	const bgColors = {
		success: "bg-green-900/90 border-green-700",
		error: "bg-red-900/90 border-red-700",
		warning: "bg-yellow-900/90 border-yellow-700",
		info: "bg-blue-900/90 border-blue-700",
	};

	return (
		<div
			className={`flex items-start gap-3 p-4 rounded-lg border shadow-lg animate-slide-in ${bgColors[toast.type] || bgColors.info}`}
		>
			{icons[toast.type] || icons.info}
			<p className="flex-1 text-sm text-gray-100">{toast.message}</p>
			<button
				onClick={() => onRemove(toast.id)}
				className="text-gray-400 hover:text-gray-200 transition-colors"
			>
				<X className="h-4 w-4" />
			</button>
		</div>
	);
}

export function useToast() {
	const context = useContext(ToastContext);
	if (!context) {
		throw new Error("useToast must be used within a ToastProvider");
	}
	return context;
}
