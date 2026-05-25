/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./src/**/*.html", "./src/**/*.templ", "./src/**/*.go", "!./src/**/*_templ.go", "!./src/**/*_gen.go"],
  safelist: [],
};
