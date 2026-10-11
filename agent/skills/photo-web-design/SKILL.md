---
name: photo-web-design
description: Compose expressive, interactive photo websites within ShutterSeek's offline sandbox.
---
# Photo website design

Start from this work's approved mood, subjects, photographs and audience. Choose a
specific visual idea before writing code. A quiet brief may stay quiet; an explicit
request for playfulness needs a meaningful interaction, not merely a small hover effect.

Create a compact internal design direction: a restrained palette drawn from the
photographs, a Chinese typography hierarchy, a contrasting rhythm of image sizes,
and one memorable interaction. Keep these working notes out of the finished page.

Think in visual scenes rather than repeating the same image-and-caption block.
Give the opening a clear focal point. Change the composition at least once when
there are several scenes: an asymmetric pairing, a horizontal filmstrip, an
oversized detail or an interactive arrangement can establish a different rhythm.
The page length must earn its space; avoid huge empty gaps between every photo.

Use local Chinese font stacks with intentional weight and scale. Choose surface,
ink and accent colors for the photographs and approved style, not the application
shell. A design can be saturated, graphic, tactile or cinematic without long copy.
Use inline SVG and CSS for simple original motifs; do not encode graphics as images.

Pick a purposeful interaction that helps the viewer explore the actual photographs.
For a playful brief, consider a small photo desk with click-to-focus and shuffle,
a filmstrip with keyboard and touch controls, a revealable memory, a chapter
navigator or a comparison slider. Give controls stable ASCII IDs and Chinese labels.
There must be a usable keyboard and touch path. A control must do what its label says.

Keep content visible even before animation begins. Honor reduced motion and make
feedback respond to user actions. Avoid scripts that depend on network requests,
storage, pointer hover alone, autoplay or long animation chains.
Load below-the-fold images lazily and decode asynchronously; keep the opening
image ready. Avoid expensive per-frame work and large decorative markup.
Never repeat rotation supplied by the photo asset service: those pixels are upright.

Use selective Chinese headings and captions. Observations are design references;
unknown dates, relationships and places are omitted without placeholder labels.
Do not manufacture a timeline or map from uncertain observations.

Before returning code, use inspect_candidate if useful to check photo references,
control IDs and forbidden resources. The executable interaction manifest identifies
one real control and optionally a visible result with simple #id selectors.
Only the workflow may render, screenshot and commit versions; model tools cannot
bypass visual checks or spend additional browser rounds.

Available tools: read_design_skill, get_design_pattern, get_photo_context and
inspect_candidate. These are read-only helpers. They provide no shell, package
installation, arbitrary file paths, network access or application credentials.
Build with native HTML/CSS/JavaScript. No React, Tailwind runtime, remote fonts,
CDNs, image generation, encoded image URLs or browser automation scripts.

Adapted for this application's runtime using original project instructions.
Design-process reference: https://github.com/anthropics/skills/tree/main/skills/frontend-design
