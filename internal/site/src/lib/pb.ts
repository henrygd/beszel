import PocketBase from "pocketbase"
import { basePath } from "@/components/router"

/**
 * Shared PocketBase client.
 * Lives in its own module so stores/api can both depend on it
 * without creating a circular import between each other.
 */
export const pb = new PocketBase(basePath)
