# 0005. Scouting list: any athlete may be discoverable, with safeguards

- Status: accepted
- Date: 2026-11-25

## Context
The owner decided that every athlete, at any age, may choose to be discoverable by coaches. A list of minors with images, open to self-registered coaches, is a real safety risk. This ADR records the decision, the safeguards and the risk that remains.

## Decision
- Discoverable is off by default. The athlete turns it on.
- Under 18: before discoverable turns on, a guardian-consent tick and the guardian's email are recorded with a timestamp.
- Coaches never see email, phone or other contact details. They send a contact request. Only on `accepted` do both sides see the details each one chose to share.
- Only signed-in coach-role accounts can see the scouting list. Anonymous visitors cannot.
- Any user can report a profile, a coach or a request. The admin can hide a profile or ban a user.
- Profiles show an age band, not a birth date.

## Options considered
- 18+ only discoverable: safest, rejected by the owner.
- Admin-verified coaches: strong, not chosen for v2. It is the first safeguard to add if abuse appears.

## Consequences and remaining risk
- Coach accounts are self-declared, so a bad actor can sign up as a coach and see minors' key frames. Reports and bans act only after the harm.
- Guardian consent is self-declared and not verified by email in v2.
- The README states these limits openly.
