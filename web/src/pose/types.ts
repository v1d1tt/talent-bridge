// The pose sequence the browser sends (GLOSSARY: "Pose sequence").
// One landmark is [x, y, visibility, world_x, world_y, world_z]:
// x and y are 0..1 of the video width and height; world values are metres
// from the midpoint of the hips. A frame with no person has visibility 0
// everywhere.

export type Landmark = [x: number, y: number, visibility: number, wx: number, wy: number, wz: number];

export type PoseSequence = {
  fps: number;
  width: number;
  height: number;
  frames: Landmark[][];
};

/** Landmarks per frame (MediaPipe Pose Landmarker). */
export const LANDMARKS = 33;

/** Index of the visibility value in a landmark. */
export const VIS = 2;
