# Glossary

The words below have one meaning everywhere in Talent-Bridge: in code, in the database, in the UI and in docs. If code needs a new domain word, add it here first.

## People and roles

| Term | Meaning |
|---|---|
| **Athlete** | A signed-in user who records clips and gets sessions. Any age from 13. |
| **Coach** | A signed-in user with the coach role. Can see the scouting list and send contact requests. |
| **Admin** | The owner. Manages references, shot gates, reports and bans. |
| **Guardian** | The parent or legal guardian of an athlete under 18. Gives guardian consent. |

## Movement and analysis

| Term | Meaning |
|---|---|
| **Clip** | A short video the athlete picks or records. It stays on the device. The server never receives it. |
| **Landmarks** | The 33 body points per frame that MediaPipe Pose Landmarker returns (normalised image x, y, plus visibility; world x, y, z in metres). |
| **Pose sequence** | The landmarks of all frames of one clip, in time order, plus the frame rate. |
| **Discipline** | `batting` or `bowling`. |
| **Shot** | One named action inside a discipline: for example `straight_drive`, `pull`, `cover_drive` (batting) or `fast_bowling`, `leg_spin` (bowling). Each shot has its own references. Shots are never mixed. |
| **View** | The camera angle of a clip: `side_on` or `front_on`. Detected from the pose. A clip with no clear view is rejected. |
| **Handedness** | `right` or `left`. Left-handed pose sequences are mirrored to the right-handed frame before comparison. |
| **Phase** | A named key moment inside a shot. Bowling: `back_foot_contact`, `front_foot_contact`, `release`. Batting: `stance`, `stride`, `contact`. |
| **Joint angle** | An angle at one joint (for example front knee, bowling-arm elbow), computed from three landmarks. |
| **Metric** | A measured value used for scoring: a joint angle at a phase, or a distance such as feet gap. Definitions follow PoseForge (arXiv 2608.05971). |
| **Reference** | The pose sequence of one pro player for one shot and one view, with player name, handedness and source. Created on the admin page with the same browser pose code as an analysis. No picture of a pro clip is stored or shown. |
| **Source** (reference) | Where a pro clip came from: a URL or a short note. Shown as a link only when it is an http(s) URL. |
| **Old CSV reference** | A reference imported once from the 2026 hackathon CSVs (`origin = legacy_csv`, ADR 0008). It has 13 body points and no world positions. It is drawn, but it does not count toward a shot gate. |
| **Reference register** | The admin page (`/admin/references`) that lists every reference in ruled rows and shows the shot gates. |
| **Not measured** (landmark) | A landmark with all six values 0: the clip's source never had that point (old CSV references). Different from a landmark the model could not see (low visibility). |
| **Reference set** | All references for one shot + view. |
| **DTW** | Dynamic time warping. Aligns two pose sequences of different speed so the same moments line up. Written by hand in Go. |
| **Fault** | A metric at a phase that is outside the reference range by more than its tolerance. Has a size and a rank. |
| **Score** | A number from 0 to 100 for one session, computed only on the server from the DTW distances and faults. |
| **Closest pro** | The reference with the smallest DTW distance to the athlete's pose sequence. |
| **Shot detection** | The server's guess of the shot (nearest reference sets by DTW distance), with a confidence. The athlete confirms or changes it. |
| **Shot gate** | The rule that a shot + view is offered to users only when its reference set has 3 or more references and passes the leave-one-pro-out test. |
| **Leave-one-pro-out test** | For each reference, score it against the other references of the same set. Each must score above the gate threshold. Runs in CI. |
| **Session** (analysis) | One analysed clip: pose sequence, shot, view, score, faults, key frames, coaching text. Stored in the `analyses` table. Not the same as an auth session. |
| **Key frame** | A still JPEG at a phase (for example release or contact), cut in the browser and stored in Azure Blob Storage. 2-3 per session. Until phases exist (slice 5), key frames are the moments of most movement. States: `pending` (upload URL given out), `uploaded` (checked: a JPEG of a sane size), `rejected` (not a valid JPEG; deleted). |
| **Pose model** | The MediaPipe Pose Landmarker model that runs in the browser: `lite` or `full`. Picked by a benchmark in slice 3. |
| **Signed URL** (SAS) | A short-lived Azure Blob Storage URL whose query string is signed by the server (a shared access signature). It allows one action on one key frame: create it (10 minutes) or read it (15 minutes). On Azure it is a user delegation SAS: signed with a key the app gets with its managed identity, so no storage account key exists. |

## Coaching

| Term | Meaning |
|---|---|
| **Coach agent** | The LLM agent (Google ADK for Go v2) that explains faults and builds a drill plan. It may use only its tools. It never computes numbers itself. |
| **Tool** (agent) | A typed Go function the coach agent can call: `get_faults`, `get_history`, `compare_to_pro`, `build_drill_plan`. |
| **Drill** | A named exercise from the fixed drill catalogue, linked to the faults it fixes. |
| **Drill plan** | 4 weeks of drills for one athlete, built only from the drill catalogue. |
| **Grounding check** | The rule that every number in the coach agent's text must equal a value from a tool result. |
| **Eval harness** | The CI test set that runs the coach agent on fixed sessions and checks grounding, drill names and fault order. |
| **Template fallback** | Fixed coaching text built from the faults, used when the model fails or fails the grounding check. |

## Scouting

| Term | Meaning |
|---|---|
| **Profile** | The athlete's public card: display name, age band, region, shots, best scores, chosen key frames. No contact details. |
| **Discoverable** | An athlete setting. When on, coaches can find the profile in the scouting list. Off by default. |
| **Guardian consent** | A record that a guardian agreed (tick + guardian email + time) before an athlete under 18 turns on discoverable. |
| **Scouting list** | The coach-only page of discoverable profiles, with filters. |
| **Contact request** | A short message from a coach to an athlete. States: `pending`, `accepted`, `declined`, `withdrawn`. On `accepted`, both see the contact details each chose to share. |
| **Report** | A flag by any user on a profile, a coach or a contact request. The admin reviews it. |
| **Hidden** | A profile the admin removed from the scouting list. |
| **Banned** | A user the admin blocked from signing in. |

## Auth

| Term | Meaning |
|---|---|
| **Auth session** | A server-side sign-in record in Postgres (`auth_sessions`), pointed to by an httpOnly cookie. It ends 7 days after its last use (idle expiry) or 30 days after sign-in (absolute expiry), whichever is first. Not the same as an analysis session. |
| **CSRF token** | A per-auth-session token that every state-changing request must send in the `X-CSRF-Token` header. |
| **Role** | `athlete`, `coach` or `admin`. A new user chooses athlete or coach once, at first sign-in (role choice). Admin is never chosen: it comes from the `ADMIN_EMAILS` setting, for a Google-verified email. |
| **Access rule** | What a route needs: public, optional (anyone; the session is used if there is one), signed in, or one of some roles. A route with no rule lets nobody in (deny by default). |
| **Fake Google** | The local OpenID Connect provider (`cmd/fake-google`) that tests and local development sign in through. The server trusts it only on localhost (ADR 0007). |
