const strings = {
  en: {
    comments: 'Comments',
    leaveComment: 'Write a comment',
    reply: 'Reply',
    loginToComment: 'Sign in to join the discussion',
    loginWithGitHub: 'Continue with GitHub',
    loginWithGoogle: 'Continue with Google',
    signIn: 'Sign in',
    signOut: 'Sign out',
    cancel: 'Cancel',
    submit: 'Post comment',
    submitting: 'Posting…',
    loadMore: 'Load more',
    awaitingApproval: 'Your comment is awaiting approval.',
    noComments: 'No comments yet. Be the first!',
    loadError: 'Failed to load comments.',
    submitError: 'Failed to post comment. Please try again.',
    commentingAs: 'Commenting as',
    close: 'Close',
    deleteComment: 'Delete',
  },
} as const

type Lang = keyof typeof strings
type Strings = (typeof strings)['en']

export function useTranslations(lang: string): Strings {
  const l = (lang in strings ? lang : 'en') as Lang
  return strings[l]
}
