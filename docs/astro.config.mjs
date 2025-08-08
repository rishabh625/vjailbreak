// @ts-check
import starlight from '@astrojs/starlight';
import { defineConfig } from "astro/config";
import AutoImport from 'astro-auto-import';
import mdx from '@astrojs/mdx';
import mermaid from "astro-mermaid";
// https://astro.build/config
export default defineConfig({
	site: process.env.SITE_URL || 'https://platform9.github.io',
	//base: '/vjailbreak/',
	base: '/' + (process.env.BASE || ''),
	trailingSlash: "always",
	integrations: [
		mermaid({
			theme: 'forest',
			autoTheme: true,
			iconPacks: [
				{
				  name: 'logos',
				  loader: () => fetch('https://unpkg.com/@iconify-json/logos@1/icons.json').then(res => res.json())
				},
			  ],
		  }),
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
						{ label: 'Prerequisites', slug: 'introduction/prerequisites' },
						{ label: 'Getting Started', slug: 'introduction/getting_started' },
					],
				},
				{
					label: 'Concepts',
					items: [
						{ label: 'Credential Management', slug: 'concepts/credential-management' },
						{ label: 'Network & Storage Mapping', slug: 'concepts/network-storage-mapping' },
						{ label: 'Migration Options', slug: 'concepts/migration-options' },
						{ label: 'Cluster Conversion', slug: 'concepts/cluster-conversion' },
					],
				},
				{
					label: 'Architecture',
					items: [
						{ label: 'Architecture', slug: 'architecture/architecture-overview' },
						{ label: 'vJailbreak VM', slug: 'architecture/vjailbreak-vm' },
						{ label: 'Components', slug: 'architecture/components' },
					],
				},
				
				{
					label: 'Guide',
					autogenerate: { directory: 'guides' },
					// items: [
					// 	{ label: 'Scaling', slug: 'guides/How-to/scaling' },
					// 	{ label: 'Troubleshooting', autogenerate: { directory: 'guides/troubleshooting' } },
					// 	{ label: 'Building', slug: 'guides/How-to/building' },
					// 	{ label: 'Using APIs', slug: 'guides/How-to/using_apis' },
					// 	{ label: 'Injecting Custom Environment Variables', slug: 'guides/How-to/injecting_custom_env' },
					// 	{ label: 'Debug Log Collection', slug: 'guides/How-to/debuglogs' },
					// 	{ label: 'Debug vJailbreak Installation', slug: 'guides/How-to/debug_vjailbreak_install' },
					// 	{ label: 'VirtIO Windows Driver Support', slug: 'guides/How-to/virtio_doc' },
					// ],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'vJailbreak CRDs', slug: 'reference/reference' },
						{ label: 'Compatibility Matrix', slug: 'reference/compatibility' },
					],
				},
				{
					label: 'Release Notes',
					autogenerate: { directory: 'release_docs' },
				},
				{
					label: 'Archives',
					autogenerate: { directory: 'archives' },
				},
			],
			customCss: [
				'./src/styles/custom.css'
				],
		}),
		mdx(),
	],
});
