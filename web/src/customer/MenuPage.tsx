import { useParams } from 'react-router';
import { Placeholder } from '../shared/Placeholder';

/** Public menu opened from a table QR code: /m/:token. */
export function MenuPage() {
  const { token } = useParams();
  return (
    <Placeholder title="Menu" phase="Phase 3, customer ordering">
      <p className="muted">Table token: {token}</p>
    </Placeholder>
  );
}
