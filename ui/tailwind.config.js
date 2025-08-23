/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: { extend: {} },
  corePlugins: { preflight: false },
  prefix: "tw-",
  plugins: [],
};
