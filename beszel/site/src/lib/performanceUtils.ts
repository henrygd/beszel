import { SystemRecord } from "@/types"

/**
 * Performance optimization utilities for large datasets
 */

/**
 * Efficiently check if any system has a specific tag using early exit
 * @param systems - Array of systems to check
 * @param tag - Tag to search for
 * @returns boolean indicating if tag exists
 */
export function hasSystemWithTag(systems: SystemRecord[], tag: string): boolean {
  for (const system of systems) {
    if (system.tags?.includes(tag)) {
      return true
    }
  }
  return false
}

/**
 * Count systems by status efficiently
 * @param systems - Array of systems
 * @returns Object with status counts
 */
export function getStatusCounts(systems: SystemRecord[]): Record<string, number> {
  const counts = { up: 0, down: 0, paused: 0, pending: 0 }
  
  for (const system of systems) {
    if (system.status in counts) {
      counts[system.status as keyof typeof counts]++
    }
  }
  
  return counts
}

/**
 * Batch operations for better performance when dealing with large datasets
 * @param items - Array of items to process
 * @param batchSize - Size of each batch
 * @param processor - Function to process each batch
 */
export async function processBatched<T, R>(
  items: T[],
  batchSize: number,
  processor: (batch: T[]) => Promise<R[]>
): Promise<R[]> {
  const results: R[] = []
  
  for (let i = 0; i < items.length; i += batchSize) {
    const batch = items.slice(i, i + batchSize)
    const batchResults = await processor(batch)
    results.push(...batchResults)
    
    // Allow other tasks to run between batches
    await new Promise(resolve => setTimeout(resolve, 0))
  }
  
  return results
}

/**
 * Memoization utility for expensive computations
 * @param fn - Function to memoize
 * @param getKey - Function to generate cache key
 */
export function memoize<TArgs extends unknown[], TReturn>(
  fn: (...args: TArgs) => TReturn,
  getKey?: (...args: TArgs) => string
): (...args: TArgs) => TReturn {
  const cache = new Map<string, TReturn>()
  
  return (...args: TArgs): TReturn => {
    const key = getKey ? getKey(...args) : JSON.stringify(args)
    
    if (cache.has(key)) {
      return cache.get(key)!
    }
    
    const result = fn(...args)
    cache.set(key, result)
    return result
  }
}

/**
 * Throttle function calls for performance optimization
 * @param func - Function to throttle
 * @param delay - Delay in milliseconds
 */
export function throttle<TArgs extends unknown[]>(
  func: (...args: TArgs) => void,
  delay: number
): (...args: TArgs) => void {
  let timeoutId: ReturnType<typeof setTimeout> | null = null
  let lastExecTime = 0
  
  return (...args: TArgs): void => {
    const currentTime = Date.now()
    
    if (currentTime - lastExecTime > delay) {
      func(...args)
      lastExecTime = currentTime
    } else {
      if (timeoutId) {
        clearTimeout(timeoutId)
      }
      
      timeoutId = setTimeout(() => {
        func(...args)
        lastExecTime = Date.now()
        timeoutId = null
      }, delay - (currentTime - lastExecTime))
    }
  }
}