import { NavLink, Link } from "react-router-dom";

/** Nav is the masthead, shared by every screen. */
export function Nav() {
  return (
    <nav className="nav">
      <Link to="/" className="nav__home">
        <img src="/assets/glass.png" alt="The Last Call — home" className="nav__mark" />
      </Link>
      <div className="nav__links">
        <NavLink to="/hours" className="nav__link">
          Hours
        </NavLink>
        <NavLink to="/find-us" className="nav__link">
          Find us
        </NavLink>
        <NavLink to="/knock" className="nav__knock">
          Knock
        </NavLink>
      </div>
    </nav>
  );
}

/** Footer carries the house rules. */
export function Footer() {
  return (
    <footer className="footer">
      <span>Doors open when the lights go down</span>
      <span className="footer__diamond" aria-hidden="true">
        ◆
      </span>
      <span>21+ · Dress like you mean it</span>
    </footer>
  );
}

/** Rule is the diamond divider under a headline. */
export function Rule() {
  return (
    <div className="rule" aria-hidden="true">
      <span className="rule__line" />
      <span className="rule__diamond">◆</span>
      <span className="rule__line" />
    </div>
  );
}

/** Masthead is the eyebrow / headline / divider stack. */
export function Masthead({ eyebrow, headline }: { eyebrow: string; headline: string }) {
  return (
    <>
      <p className="eyebrow">{eyebrow}</p>
      <h1 className="headline">{headline}</h1>
      <Rule />
    </>
  );
}

/** Screen wraps a page in the room's gradient, masthead and footer. */
export function Screen({ children }: { children: React.ReactNode }) {
  return (
    <div className="page">
      <Nav />
      <main className="main">{children}</main>
      <Footer />
    </div>
  );
}
