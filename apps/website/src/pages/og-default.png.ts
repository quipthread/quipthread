import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { Resvg } from '@resvg/resvg-js'
import type { APIRoute } from 'astro'
import satori from 'satori'

const fontData = await readFile(join(process.cwd(), 'public', 'fonts', 'syne-700.ttf'))

export const GET: APIRoute = async () => {
  const svg = await satori(
    {
      type: 'div',
      props: {
        style: {
          width: '1200px',
          height: '630px',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'center',
          alignItems: 'flex-start',
          background: '#1A1714',
          padding: '64px 80px',
          fontFamily: 'Syne',
        },
        children: [
          {
            type: 'div',
            props: {
              style: {
                fontSize: 72,
                fontWeight: 700,
                color: '#E8E3DC',
                letterSpacing: '-0.02em',
                lineHeight: 1.1,
                marginBottom: 28,
              },
              children: 'Quipthread',
            },
          },
          {
            type: 'div',
            props: {
              style: {
                fontSize: 30,
                color: '#8A8480',
                lineHeight: 1.45,
                maxWidth: 860,
              },
              children: 'The comment system your readers will actually use.',
            },
          },
          {
            type: 'div',
            props: {
              style: {
                marginTop: 56,
                display: 'flex',
                gap: 12,
                alignItems: 'center',
              },
              children: [
                {
                  type: 'div',
                  props: {
                    style: {
                      padding: '10px 24px',
                      background: '#E07F32',
                      borderRadius: 8,
                      fontSize: 20,
                      fontWeight: 700,
                      color: '#ffffff',
                    },
                    children: 'Open source',
                  },
                },
                {
                  type: 'div',
                  props: {
                    style: {
                      padding: '10px 24px',
                      background: '#2A2724',
                      borderRadius: 8,
                      fontSize: 20,
                      fontWeight: 700,
                      color: '#8A8480',
                    },
                    children: 'quipthread.com',
                  },
                },
              ],
            },
          },
        ],
      },
    },
    {
      width: 1200,
      height: 630,
      fonts: [
        {
          name: 'Syne',
          data: fontData,
          weight: 700,
          style: 'normal',
        },
      ],
    },
  )

  const resvg = new Resvg(svg, { fitTo: { mode: 'width', value: 1200 } })
  const png = resvg.render().asPng()

  return new Response(png, {
    headers: {
      'Content-Type': 'image/png',
      'Cache-Control': 'public, max-age=31536000, immutable',
    },
  })
}
