import { createPortal } from "react-dom";

// The main content wrapper in Layout carries `relative z-10`, which makes it a
// stacking context. Anything rendered inside it is painted below the desktop
// sidebar (z-[100]) no matter how high its own z-index goes, so a modal wide
// enough to reach under the sidebar loses that strip of itself. Portalling to
// the body takes the modal out of that stacking context; the overlay then only
// has to out-rank the sidebar, which the z-[120] tier does.
const ModalPortal = ({ children }) => createPortal(children, document.body);

export default ModalPortal;
