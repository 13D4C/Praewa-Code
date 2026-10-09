export function pose(left, right, offset = 0, scale = 1) {
  const points = Array.from({ length: 33 }, () => ({
    x: 0.5,
    y: 0.8 + offset,
    visibility: 0.99,
    presence: 0.99,
  }));
  for (const index of [11, 13, 15]) points[index].x = 0.7;
  for (const index of [12, 14, 16]) points[index].x = 0.3;
  points[11].y = points[12].y = 0.6 - 0.25 * scale + offset;
  points[13].y = points[14].y = 0.6 + offset;
  points[15].y = left + offset;
  points[16].y = right + offset;
  return points;
}
