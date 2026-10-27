// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

import eslintPluginAstro from "eslint-plugin-astro";
import eslintConfigPrettier from "eslint-config-prettier";

export default [
	...eslintPluginAstro.configs.recommended,
	eslintConfigPrettier,
	{
		ignores: ["dist/", ".astro/"],
	},
];
