import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const root = path.resolve(process.cwd(), 'src/shared/i18n/resources');
const locales = ['ru', 'en'];

function read(locale) {
  return JSON.parse(fs.readFileSync(path.join(root, `${locale}.json`), 'utf8'));
}

function flatten(value, prefix = '') {
  const result = new Map();
  for (const [key, item] of Object.entries(value)) {
    const next = prefix ? `${prefix}.${key}` : key;
    if (item && typeof item === 'object' && !Array.isArray(item)) {
      for (const [childKey, childValue] of flatten(item, next)) {
        result.set(childKey, childValue);
      }
    } else {
      result.set(next, item);
    }
  }
  return result;
}

const maps = Object.fromEntries(locales.map((locale) => [locale, flatten(read(locale))]));
let failed = false;

for (const locale of locales) {
  for (const [key, value] of maps[locale]) {
    if (typeof value !== 'string' || value.trim() === '') {
      console.error(`${locale}: empty or non-string translation for ${key}`);
      failed = true;
    }
  }
}

for (const left of locales) {
  for (const right of locales) {
    if (left === right) continue;
    for (const key of maps[left].keys()) {
      if (!maps[right].has(key)) {
        console.error(`${right}: missing key ${key}`);
        failed = true;
      }
    }
  }
}

if (failed) process.exit(1);
console.log(`i18n key parity ok: ${[...maps.ru.keys()].length} keys`);
