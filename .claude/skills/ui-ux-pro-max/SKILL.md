---
name: ui-ux-pro-max
description: Query UI/UX style, color palette, font pairing, chart type, and UX guidelines from a searchable design intelligence database. Use when building frontend components, picking design tokens, or evaluating UI patterns.
metadata:
  trigger: Building frontend UI, picking design tokens, choosing chart types, evaluating layout options
  source: nextlevelbuilder/ui-ux-pro-max-skill
  license: MIT
---

# UI/UX Pro Max

Searchable design intelligence database for style, palette, typography, chart types, and UX guidelines.

## Data Sources

The skill data lives in `data/` — query these files directly.

| File | Contents |
|------|----------|
| `data/colors.csv` | 29 product-style color systems (primary, secondary, accent, background, etc.) with WCAG notes |
| `data/charts.csv` | Chart type recommendations by data shape |
| `data/google-fonts.csv` | Font pairing recommendations |
| `data/icons.csv` | Icon library recommendations |
| `data/app-interface.csv` | App interface patterns |
| `data/landing.csv` | Landing page component patterns |
| `data/motion.csv` | Motion/animation patterns |
| `data/products.csv` | Product design patterns |
| `data/react-performance.csv` | React performance patterns |
| `data/stacks/` | Tech-stack specific patterns |

## Usage

When user describes a design need, consult the appropriate data file.

**Example:** User says "I need a dashboard color scheme"
→ Read `data/colors.csv`, find rows where Product Type matches, recommend palette.

**Example:** User says "what font for a SaaS app"
→ Read `data/google-fonts.csv`, find SaaS rows, return font pairing.

**Example:** User says "best chart for time-series data"
→ Read `data/charts.csv`, find time-series category.

## Design System Generation

To generate a project design system:

```bash
mkdir -p design-system
# Copy relevant rows from colors.csv as design tokens
# Use typography from google-fonts.csv
# Reference chart choices from charts.csv
```

The skill generates a `design-system/` folder structure with tokens, typography, and component guidelines.

## Integration with DESIGN.md

This skill provides design *reference* and *generation* capability. The output goes into `design/DESIGN.md`, which is owned by UI/UX Designer Agent. Frontend Engineer Agent consumes DESIGN.md but does not edit it.

## License

MIT