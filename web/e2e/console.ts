// Console errors a page may log without a test failing. Only one kind:
// MediaPipe tries to send usage logs to Google, and our CSP (connect-src
// 'self') blocks the call. The block is the privacy control working, so
// it is expected; it fires only on slow runs (about a minute in).
const MEDIAPIPE_TELEMETRY = "https://odml.pa.googleapis.com/";

export function unexpected(errors: string[]): string[] {
  return errors.filter((e) => !(e.includes(MEDIAPIPE_TELEMETRY) && /Content Security Policy/.test(e)));
}
