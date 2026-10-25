// Package graphcanvas implements an ultra-high-performance infinite canvas and
// knowledge graph visualization widget for Gio UI, capable of smoothly rendering
// and interacting with 100,000+ nodes and edges at 60-120 FPS.
//
// Key architectural pillars:
//   - Zero-allocation flat memory layout (DOD/SOA) eliminating Go GC pauses.
//   - Uniform 2D spatial grid partitioning (32x32) with viewport frustum culling.
//   - Persistent op.Ops macro caching with O(1) dynamic cross-chunk re-indexing.
//   - Quad-LOD visual pyramid: 2048-baked overview map -> cluster hubs -> detail vectors.
//   - Localized active-set spring physics with organic micro-breathing tension.
//   - Screen-space collision-free Chinese label pill rendering.
package graphcanvas
