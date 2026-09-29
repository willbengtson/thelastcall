import { Navigate, Route, Routes } from "react-router-dom";
import { BookABay } from "./pages/BookABay";
import { ComingSoon } from "./pages/ComingSoon";

/** App routes the site. Only `Book a Bay` is built out so far. */
export function App() {
  return (
    <Routes>
      <Route path="/" element={<BookABay />} />
      <Route path="/book" element={<Navigate to="/" replace />} />
      <Route path="/hours" element={<ComingSoon eyebrow="When" headline="Hours" />} />
      <Route path="/find-us" element={<ComingSoon eyebrow="Where" headline="Find us" />} />
      <Route path="/knock" element={<ComingSoon eyebrow="Entry" headline="Knock" />} />
      <Route
        path="*"
        element={<ComingSoon eyebrow="Lost" headline="Wrong door" />}
      />
    </Routes>
  );
}
