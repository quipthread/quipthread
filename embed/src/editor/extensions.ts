import Highlight from '@tiptap/extension-highlight'
import Link from '@tiptap/extension-link'
import Placeholder from '@tiptap/extension-placeholder'
import Subscript from '@tiptap/extension-subscript'
import Superscript from '@tiptap/extension-superscript'
import TextAlign from '@tiptap/extension-text-align'
import StarterKit from '@tiptap/starter-kit'

export function isCommentLink(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === 'https:' || url.protocol === 'http:'
  } catch (error) {
    if (error instanceof TypeError) return false
    throw error
  }
}

export function createCommentExtensions({
  placeholder,
  openLink,
}: {
  readonly placeholder: string
  readonly openLink: () => void
}) {
  return [
    StarterKit.configure({ link: false, trailingNode: false }),
    Link.extend({
      addKeyboardShortcuts() {
        return {
          'Mod-k': () => {
            openLink()
            return true
          },
        }
      },
    }).configure({
      openOnClick: false,
      defaultProtocol: 'https',
      isAllowedUri: (url, context) => context.defaultValidate(url) && isCommentLink(url),
      shouldAutoLink: isCommentLink,
    }),
    Placeholder.configure({ placeholder }),
    Highlight.configure({ multicolor: false }),
    Subscript,
    Superscript,
    TextAlign.configure({ types: ['heading', 'paragraph'] }),
  ]
}
