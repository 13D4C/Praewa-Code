import { build } from 'esbuild';
import { cp, mkdir } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';

const require = createRequire(import.meta.url);
const entry = require.resolve('@mediapipe/tasks-vision');
const directory = new URL('../public/vision/', import.meta.url);
await mkdir(directory, { recursive: true });
await cp(join(dirname(entry), 'wasm'), new URL('wasm/', directory), {
  recursive: true,
});
await build({
  entryPoints: [entry],
  outfile: new URL('vision.js', directory).pathname,
  bundle: true,
  format: 'iife',
  globalName: 'PraewaVision',
  minify: true,
  legalComments: 'eof',
});
