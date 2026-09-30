import type { ImageMeta } from "./spec.ts";

export const declaredImages: Record<string, ImageMeta> = {
  "httpd": {
    "entrypoint": [
      "httpd"
    ],
    "cmd": [
      "-f",
      "-p",
      "8080",
      "-h",
      "/www"
    ],
    "workingDir": "",
    "user": "",
    "env": [
      "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    ]
  }
};
