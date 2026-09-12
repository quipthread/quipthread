import { useLayoutEffect, useRef, useState } from 'react'

export function useToolbarOverflow(groupSizes: readonly number[]) {
  const toolbarRef = useRef<HTMLFieldSetElement>(null)
  const [visibleGroups, setVisibleGroups] = useState(groupSizes.length)

  useLayoutEffect(() => {
    const toolbar = toolbarRef.current
    if (!toolbar) return
    const measure = () => {
      const style = getComputedStyle(toolbar)
      const control = Number.parseFloat(style.getPropertyValue('--qt-editor-control'))
      const divider = Number.parseFloat(style.getPropertyValue('--qt-editor-divider'))
      const gap = Number.parseFloat(style.columnGap)
      const available =
        toolbar.clientWidth -
        Number.parseFloat(style.paddingLeft) -
        Number.parseFloat(style.paddingRight)
      const widths = groupSizes.map((size, index) => size * control + (index ? divider + gap : 0))
      if (widths.reduce((sum, width) => sum + width, 0) <= available) {
        setVisibleGroups(groupSizes.length)
        return
      }
      const overflowWidth = control + divider + gap
      let used = 0
      let count = 0
      for (const width of widths) {
        if (used + width + overflowWidth > available) break
        used += width
        count += 1
      }
      setVisibleGroups(count)
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(toolbar)
    return () => observer.disconnect()
  }, [groupSizes])

  return { toolbarRef, visibleGroups }
}
