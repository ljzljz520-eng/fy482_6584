import { cp, mkdir, rm, stat } from "node:fs/promises";

const source = new URL("./src/", import.meta.url);
const output = new URL("./dist/", import.meta.url);

await stat(new URL("index.html", source));
await rm(output, { recursive: true, force: true });
await mkdir(output, { recursive: true });
await cp(source, output, { recursive: true });
console.log("Built web/dist");
