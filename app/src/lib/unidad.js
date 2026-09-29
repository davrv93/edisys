/** «201» → «Dpto 201»; «Dpto 201» o «Estac. 3» se dejan igual. El API manda el código pelado. */
export function nombreUnidad(u) {
  if (u === null || u === undefined || u === '') return '';
  const s = String(u);
  return /^\d+[A-Za-z]?$/.test(s) ? `Dpto ${s}` : s;
}
