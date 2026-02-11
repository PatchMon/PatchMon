# PatchMon Documentation

This directory contains the Jekyll-based documentation site using the [Just the Docs](https://github.com/just-the-docs/just-the-docs) theme.

## Navigation

The sidebar navigation is defined in `_data/navigation.yml`. Edit that file to customize the sidebar structure.

## Local Development

1. Install dependencies:
   ```bash
   bundle install
   ```

2. Serve locally:
   ```bash
   bundle exec jekyll serve
   ```

3. Open `http://localhost:4000`

## GitHub Pages Setup

1. Go to repository Settings → Pages
2. Select "Deploy from a branch"
3. Choose your branch (usually `main`)
4. Select `/docs` folder
5. Click Save

GitHub Pages will automatically build and serve your documentation.

## Adding Pages

1. Create a `.md` file in the appropriate directory
2. Add front matter:
   ```yaml
   ---
   layout: default
   title: Your Page Title
   nav_order: 1
   ---
   ```
3. Add the page to `_data/navigation.yml` if you want it in the sidebar

