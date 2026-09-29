import { Link } from "react-router-dom";
import { Masthead, Screen } from "../components/Chrome";

/**
 * ComingSoon holds the place of a screen that exists in the design but has not
 * been built yet, so nav never leads anywhere dead.
 */
export function ComingSoon({ eyebrow, headline }: { eyebrow: string; headline: string }) {
  return (
    <Screen>
      <Masthead eyebrow={eyebrow} headline={headline} />
      <p className="lede">Still being poured. Come back before last call.</p>
      <div className="actions">
        <Link to="/" className="btn-outline">
          Claim a bay
        </Link>
      </div>
    </Screen>
  );
}
