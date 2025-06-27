// @ts-check
import starlight from '@astrojs/starlight';
import { defineConfig } from "astro/config";
import AutoImport from 'astro-auto-import';
import mdx from '@astrojs/mdx';
// https://astro.build/config
export default defineConfig({
	site: process.env.SITE_URL || 'https://platform9.github.io',
	//base: '/vjailbreak/',
	base: '/' + (process.env.BASE || ''),
	trailingSlash: "always",
	integrations: [
		AutoImport({
			imports: [
			  './src/components/ReadMore.astro',
			],
		  }),
		starlight({
			title: 'vJailbreak',
			editLink: {
				baseUrl: 'https://platform9.github.io/vjailbreak/',
			},
			social: {
				github: 'https://github.com/platform9/vjailbreak',
				slack: 'https://join.slack.com/t/vjailbreak/shared_invite/zt-314pppw43-F1vzd6ZaPW5PoZqF~aa8lA',
			},
			plugins: [],
			components: {
				Header: './src/components/Header.astro',
				//SocialIcons: './src/components/githubRelease.astro',
			},
			logo: {
				src: './src/assets/platform9-logo.svg',
				replacesTitle: true,
			},
			head: [
			],
			sidebar: [
				{
					label: 'Introduction',
					items: [
						// Each item here is one entry in the navigation menu.
						// manually done so to we can keep the order
						{ label: 'What is vJailbreak', slug: 'introduction/what_is_vjailbreak' },
						{ label: 'Getting Started', slug: 'introduction/getting_started' },
						{ label: 'Prerequisites', slug: 'introduction/prerequisites' },
						{ label: 'Components', slug: 'introduction/components' },
					],
				},
				{
					label: 'Guide',
					//autogenerate: { directory: 'guides' },
					items: [
						{ label: 'Scaling', slug: 'guides/scaling' },
						{ label: 'Troubleshooting', slug: 'guides/troubleshooting' },
						{ label: 'Building', slug: 'guides/building' },
						{ label: 'Using APIs', slug: 'guides/using_apis' },
						{ label: 'Injecting Custom Environment Variables', slug: 'guides/injecting_custom_env' },
						{ label: 'Debug Log Collection', slug: 'guides/debuglogs' },
						{ label: 'Debug vJailbreak Installation', slug: 'guides/debug_vjailbreak_install' },
						{ label: 'User-Provided VirtIO Windows Driver Support', slug: 'guides/virtio_doc' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'vJailbreak CRDs', slug: 'reference/reference' },
					],
				},
				{
					label: 'Release Notes',
					autogenerate: { directory: 'release_docs' },
				},
			],
			customCss: [
				'./src/styles/custom.css'
				],
		}),
		mdx(),
	],
});
