/**
 * The `files` option: the service passes the aerial clones as `file://`
 * URLs. Anything else is dropped, so a stray value can never point the
 * video at the network.
 */
export function fileList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === "string" && item.startsWith("file://"));
}

/** A shuffled copy of `items` (Fisher-Yates). `random` returns 0 <= n < 1. */
export function shuffled<T>(items: readonly T[], random: () => number): T[] {
  const copy = [...items];
  for (let index = copy.length - 1; index > 0; index--) {
    const other = Math.floor(random() * (index + 1));
    [copy[index], copy[other]] = [copy[other], copy[index]];
  }
  return copy;
}

export interface Playlist {
  /** The next file to play, or undefined when there is none. */
  next(): string | undefined;
}

/**
 * Plays every file once in a shuffled order, then shuffles again. A new
 * round never starts with the file that ended the last one, so with two or
 * more files the same aerial never plays twice in a row.
 */
export function createPlaylist(files: readonly string[], random: () => number): Playlist {
  let round: string[] = [];
  let last: string | undefined;
  return {
    next() {
      if (files.length === 0) return undefined;
      if (round.length === 0) {
        round = shuffled(files, random);
        if (round.length > 1 && round[0] === last) round.push(round.shift() as string);
      }
      last = round.shift();
      return last;
    },
  };
}
