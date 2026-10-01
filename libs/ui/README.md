# @waypoint/ui

Components shared across all four role workspaces, plus the design tokens as TypeScript
values.

Only genuinely cross-role pieces belong here. A screen used by one role lives in that role's
folder under `apps/web/src/app`.

| Export                                            | Use                                            |
| ------------------------------------------------- | ---------------------------------------------- |
| `StatusBadge`, `OrderStatusBadge`                 | Status with a glyph as well as a colour        |
| `Mono`, `ClockTimeText`, `MinuteDelta`            | IDs and 24-hour times in tabular figures       |
| `OfflineBanner`, `useOnlineStatus`                | Connectivity, rendered neutral rather than red |
| `COLORS`, `BRAND_COLORS`, `BREAKPOINTS`, `LAYOUT` | Tokens for canvas, charts and generated SVG    |

## Two rules from the Day 5 style guide

**Status is never signalled by colour alone.** `StatusBadge` pairs every tone with a glyph, so
it reads correctly in greyscale, on a dock tablet in daylight, and for a colour-blind loader.

**Offline is neutral, never red.** Losing signal on a hill-country route is an expected
condition, not a fault the driver caused.

## Tokens

The CSS custom properties in `apps/web/src/app/global.css` are the source of truth. The
constants in `tokens.ts` exist only for places CSS variables cannot reach — canvas, chart
libraries, generated SVG, `theme-color`. Keep the two in step.

If you add a component here that uses a new Tailwind class, check it compiles: Tailwind v4
finds this library only because `global.css` names it in an `@source` line.

```bash
npx nx test ui
```
