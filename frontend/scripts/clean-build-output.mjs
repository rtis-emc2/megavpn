import { rm } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const distRoot = join(scriptDir, '..', 'dist');

await rm(distRoot, { force: true, recursive: true });
