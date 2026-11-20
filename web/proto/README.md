# PROTOTYPE: app shell and Proxy Host form (throwaway)

Answers wayfinder ticket "App shell and Proxy Host form look". Not production code; never merge to main.

```bash
cd web/proto && pnpm install && pnpm prototype
```

Open `http://localhost:5173/?variant=A|B|C` (arrow keys or the bottom bar switch). Mock data, nothing persists.

- A: sidebar, dense table, form in a slide-over sheet
- B: top nav, health summary cards, inline tabbed edit, errors shown on the card
- C: icon rail, master/detail, live "what will happen" summary beside the form
