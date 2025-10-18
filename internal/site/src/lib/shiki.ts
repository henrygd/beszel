// https://shiki.style/guide/bundles#fine-grained-bundle

// directly import the theme and language modules, only the ones you imported will be bundled.
import githubDarkDimmed from '@shikijs/themes/github-dark-dimmed'

// `shiki/core` entry does not include any themes or languages or the wasm binary.
import { createHighlighterCore } from 'shiki/core'
import { createOnigurumaEngine } from 'shiki/engine/oniguruma'

export const highlighter = await createHighlighterCore({
   themes: [
      // instead of strings, you need to pass the imported module
      githubDarkDimmed,
      // or a dynamic import if you want to do chunk splitting
      //  import('@shikijs/themes/material-theme-ocean')
   ],
   langs: [
      import('@shikijs/langs/log'),
      import('@shikijs/langs/json'),
      // shiki will try to interop the module with the default export
      // () => import('@shikijs/langs/css'),
   ],
   // `shiki/wasm` contains the wasm binary inlined as base64 string.
   engine: createOnigurumaEngine(import('shiki/wasm'))
})

// optionally, load themes and languages after creation
// await highlighter.loadTheme(import('@shikijs/themes/vitesse-light'))
