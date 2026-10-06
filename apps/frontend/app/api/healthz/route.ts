import { liveness } from '../../../lib/ops';

export const dynamic = 'force-dynamic';

export function GET() {
  return liveness();
}
