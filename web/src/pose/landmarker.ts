// Loads MediaPipe Pose Landmarker (ADR 0001) from this app's own origin.
// The files are copied into public/mediapipe by scripts/mediapipe.mjs.
// This module is imported lazily, only on the pages that read a clip.
import { FilesetResolver, PoseLandmarker } from "@mediapipe/tasks-vision";

export type ModelName = "lite" | "full";
export type Delegate = "GPU" | "CPU";

/** The model the app uses. Picked by the slice 3 benchmark (docs/learning/03.md). */
export const DEFAULT_MODEL: ModelName = "full";

const base = "/mediapipe";

export const modelPath = (m: ModelName) => `${base}/models/pose_landmarker_${m}_float16_1.task`;

export type Loaded = {
  landmarker: PoseLandmarker;
  model: ModelName;
  delegate: Delegate;
  /** Next timestamp to use: VIDEO mode needs timestamps that only grow. */
  nextTimestampMs: number;
};

/** Loads the model, on the GPU when the browser allows it, else on the CPU. */
export async function loadLandmarker(model: ModelName = DEFAULT_MODEL): Promise<Loaded> {
  const fileset = await FilesetResolver.forVisionTasks(`${base}/${__MEDIAPIPE_VERSION__}/wasm`);
  const create = (delegate: Delegate) =>
    PoseLandmarker.createFromOptions(fileset, {
      baseOptions: { modelAssetPath: modelPath(model), delegate },
      runningMode: "VIDEO",
      numPoses: 1,
    });
  try {
    return { landmarker: await create("GPU"), model, delegate: "GPU", nextTimestampMs: 0 };
  } catch {
    return { landmarker: await create("CPU"), model, delegate: "CPU", nextTimestampMs: 0 };
  }
}
