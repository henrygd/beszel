import { SystemRecord } from "@/types"
import { pb } from "./api"

/**
 * Bulk operations for tags and groups
 */

export interface BulkRenameResult {
  success: boolean
  affectedSystems: number
  errors?: string[]
}

/**
 * Rename a tag across all systems that use it
 * @param oldTag - The tag name to replace
 * @param newTag - The new tag name
 * @param systems - Array of all systems
 * @returns Result of the bulk operation
 */
export async function bulkRenameTag(
  oldTag: string,
  newTag: string,
  systems: SystemRecord[]
): Promise<BulkRenameResult> {
  const affectedSystems = systems.filter(system => 
    system.tags?.includes(oldTag)
  )
  
  if (affectedSystems.length === 0) {
    return { success: true, affectedSystems: 0 }
  }

  const errors: string[] = []
  let successCount = 0

  // Process in batches for better performance
  const batchSize = 10
  for (let i = 0; i < affectedSystems.length; i += batchSize) {
    const batch = affectedSystems.slice(i, i + batchSize)
    
    const batchPromises = batch.map(async (system) => {
      try {
        const updatedTags = system.tags?.map(tag => 
          tag === oldTag ? newTag : tag
        ) || []
        
        await pb.collection("systems").update(system.id, { tags: updatedTags })
        successCount++
      } catch (error) {
        errors.push(`Failed to update system ${system.name}: ${error}`)
      }
    })
    
    await Promise.all(batchPromises)
    
    // Small delay between batches to prevent overwhelming the server
    if (i + batchSize < affectedSystems.length) {
      await new Promise(resolve => setTimeout(resolve, 100))
    }
  }

  return {
    success: errors.length === 0,
    affectedSystems: successCount,
    errors: errors.length > 0 ? errors : undefined
  }
}

/**
 * Rename a group across all systems that use it
 * @param oldGroup - The group name to replace
 * @param newGroup - The new group name
 * @param systems - Array of all systems
 * @returns Result of the bulk operation
 */
export async function bulkRenameGroup(
  oldGroup: string,
  newGroup: string,
  systems: SystemRecord[]
): Promise<BulkRenameResult> {
  const affectedSystems = systems.filter(system => 
    system.group === oldGroup
  )
  
  if (affectedSystems.length === 0) {
    return { success: true, affectedSystems: 0 }
  }

  const errors: string[] = []
  let successCount = 0

  // Process in batches for better performance
  const batchSize = 10
  for (let i = 0; i < affectedSystems.length; i += batchSize) {
    const batch = affectedSystems.slice(i, i + batchSize)
    
    const batchPromises = batch.map(async (system) => {
      try {
        await pb.collection("systems").update(system.id, { group: newGroup })
        successCount++
      } catch (error) {
        errors.push(`Failed to update system ${system.name}: ${error}`)
      }
    })
    
    await Promise.all(batchPromises)
    
    // Small delay between batches to prevent overwhelming the server
    if (i + batchSize < affectedSystems.length) {
      await new Promise(resolve => setTimeout(resolve, 100))
    }
  }

  return {
    success: errors.length === 0,
    affectedSystems: successCount,
    errors: errors.length > 0 ? errors : undefined
  }
}

/**
 * Delete a tag from all systems that use it
 * @param tagToDelete - The tag to remove
 * @param systems - Array of all systems
 * @returns Result of the bulk operation
 */
export async function bulkDeleteTag(
  tagToDelete: string,
  systems: SystemRecord[]
): Promise<BulkRenameResult> {
  const affectedSystems = systems.filter(system => 
    system.tags?.includes(tagToDelete)
  )
  
  if (affectedSystems.length === 0) {
    return { success: true, affectedSystems: 0 }
  }

  const errors: string[] = []
  let successCount = 0

  // Process in batches for better performance
  const batchSize = 10
  for (let i = 0; i < affectedSystems.length; i += batchSize) {
    const batch = affectedSystems.slice(i, i + batchSize)
    
    const batchPromises = batch.map(async (system) => {
      try {
        const updatedTags = system.tags?.filter(tag => tag !== tagToDelete) || []
        await pb.collection("systems").update(system.id, { tags: updatedTags })
        successCount++
      } catch (error) {
        errors.push(`Failed to update system ${system.name}: ${error}`)
      }
    })
    
    await Promise.all(batchPromises)
    
    // Small delay between batches to prevent overwhelming the server
    if (i + batchSize < affectedSystems.length) {
      await new Promise(resolve => setTimeout(resolve, 100))
    }
  }

  return {
    success: errors.length === 0,
    affectedSystems: successCount,
    errors: errors.length > 0 ? errors : undefined
  }
}

/**
 * Get systems that would be affected by a tag rename
 * @param tagName - The tag to check
 * @param systems - Array of all systems
 * @returns Array of affected systems
 */
export function getSystemsWithTag(tagName: string, systems: SystemRecord[]): SystemRecord[] {
  return systems.filter(system => system.tags?.includes(tagName))
}

/**
 * Get systems that would be affected by a group rename
 * @param groupName - The group to check
 * @param systems - Array of all systems
 * @returns Array of affected systems
 */
export function getSystemsWithGroup(groupName: string, systems: SystemRecord[]): SystemRecord[] {
  return systems.filter(system => system.group === groupName)
}