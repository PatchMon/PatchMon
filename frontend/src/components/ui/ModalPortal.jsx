import { createPortal } from "react-dom";

// Layout's z-10 content wrapper is a stacking context; see UI_DESIGN_SYSTEM.md §9.5.
const ModalPortal = ({ children }) => createPortal(children, document.body);

export default ModalPortal;
