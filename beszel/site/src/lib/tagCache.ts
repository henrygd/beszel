import { SystemRecord } from "@/types"

// WeakMap cache for tag computations to improve performance
const tagCache = new WeakMap<SystemRecord[], string[]>()
const tagSetCache = new WeakMap<SystemRecord[], Set<string>>()

/**
 * Efficiently extract all unique tags from a list of systems using WeakMap caching
 * @param systems - Array of SystemRecord objects
 * @returns Sorted array of unique tags
 */
export function getSystemTags(systems: SystemRecord[]): string[] {
  // Check if we already computed tags for this exact systems array
  const cached = tagCache.get(systems)
  if (cached) return cached
  
  // Check if we have a cached Set for this systems array
  let tagSet = tagSetCache.get(systems)
  
  if (!tagSet) {
    // Create new tag set
    tagSet = new Set<string>()
    
    for (const system of systems) {
      if (system.tags) {
        for (const tag of system.tags) {
          if (tag) tagSet.add(tag)
        }
      }
    }
    
    // Cache the Set for future use
    tagSetCache.set(systems, tagSet)
  }
  
  // Convert to sorted array and cache it
  const result = Array.from(tagSet).sort()
  tagCache.set(systems, result)
  
  return result
}

/**
 * Clear the tag cache (useful when systems list changes significantly)
 */
export function clearTagCache(): void {
  // WeakMaps will automatically clean up when references are gone
  // This is mainly for manual cache clearing if needed
}