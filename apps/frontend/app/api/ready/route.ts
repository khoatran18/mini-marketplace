import { readiness } from '../../../lib/ops';

export const dynamic = 'force-dynamic';

export async function GET() {
  return readiness();
}
