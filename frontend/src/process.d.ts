// Minimal process.env declaration for playwright.config.ts
// Avoids pulling in all of @types/node which breaks TS 4.6.4
declare const process: {
    env: { [key: string]: string | undefined };
    CI?: string;
  };