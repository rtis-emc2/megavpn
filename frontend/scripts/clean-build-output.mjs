import { rm, readdir } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const webRoot = join(scriptDir, '..', '..', 'web');
const assetsDir = join(webRoot, 'assets');

await Promise.allSettled([
  rm(join(webRoot, 'index.html'), { force: true }),
  rm(join(webRoot, '.vite', 'manifest.json'), { force: true }),
]);

let assetNames;
try {
  assetNames = await readdir(assetsDir);
} catch (error) {
  if (error && typeof error === 'object' && 'code' in error && error.code === 'ENOENT') {
    assetNames = [];
  } else {
    throw error;
  }
}

await Promise.all(
  assetNames
    .filter((name) => /^index-[A-Za-z0-9_-]+\.(js|css)$/.test(name))
    .map((name) => rm(join(assetsDir, name), { force: true })),
);
