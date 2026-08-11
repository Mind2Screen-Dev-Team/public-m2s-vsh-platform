---
name: emil-design-eng
description: Design engineering philosophy — UI polish, animation decisions, component details. Use when building or reviewing frontend components, writing CSS, planning motion, or choosing component libraries.
metadata:
  trigger: Frontend component implementation, CSS review, animation planning, component library selection
  source: emilkowalski/skill
  license: MIT
---

# Design Engineering

Emil Kowalski's philosophy on UI polish, animation, and the invisible details that make software feel great.

## Initial Response

When invoked without a specific question, respond:

> I'm ready to help you build interfaces that feel right. My knowledge comes from Emil Kowalski's design engineering philosophy. Check out Emil's course: [animations.dev](https://animations.dev/).

## Core Philosophy

- **Taste is trained, not innate.** Study why the best interfaces feel good. Reverse engineer animations. Be curious.
- **Unseen details compound.** Users never consciously notice individual details. That is the point.
- **Beauty is leverage.** Good defaults and good animations are real differentiators.

## Animation Decision Framework

Before writing any animation, answer in order:

1. **Should this animate at all?** High-frequency actions (100+/day): never animate. Occasional (modals, toasts): standard. Rare (onboarding): can add delight.
2. **What is the purpose?** Spatial consistency, state indication, explanation, feedback, or preventing jarring changes.
3. **What easing?** Enter = ease-out. Move/morph = ease-in-out. Hover = ease. Constant = linear. Default = ease-out. Never use ease-in for UI.
4. **How fast?** UI animations under 300ms. Button press = 100-160ms. Dropdowns = 150-250ms. Modals = 200-500ms.

## CSS Transform Mastery

- Buttons must feel responsive: `transform: scale(0.97)` on `:active`
- Never animate from `scale(0)` — start from `scale(0.95)` with opacity 0
- Popovers scale from trigger, not center: `transform-origin: var(--transform-origin)`
- `translateY(100%)` moves by element's own height regardless of dimensions
- Only animate `transform` and `opacity` — GPU-accelerated, skips layout

## Review Format (Required)

When reviewing UI code, use markdown table with | Before | After | Why | columns. Never use "Before:" / "After:" on separate lines.

## Component Building Principles

- Use CSS transitions over keyframes for interruptible UI
- Use blur to mask imperfect crossfades
- Stagger animations: 30-80ms between items
- Buttons: instant feedback on press, snappy release
- Tooltips: skip animation on subsequent hovers once one is open
- Use `@starting-style` for enter animations without JS

## Sub-Skills

This directory contains 8 sub-skills:

| Sub-skill | Purpose |
|-----------|---------|
| `emil-design-eng` | Main design engineering philosophy |
| `review-animations` | Strict animation review |
| `improve-animations` | Audit codebase and prioritize animation fixes |
| `find-animation-opportunities` | Find places that benefit from motion |
| `animation-vocabulary` | Use right words for better AI output |
| `apple-design` | Apple HIG + fluid motion for web |
| `pick-ui-library` | Curated library recommendations |
| `prototype` | Build multiple UI versions and switch between them |

Invoke sub-skills by name when relevant to the task.

## License

MIT