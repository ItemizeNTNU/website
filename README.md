# Itemize Website
This is the website for itemize.no. The website uses [Svelte](https://svelte.dev) and the [Sapper](https://sapper.svelte.dev) frameworks. The website has a Dockerfile that the website runs from. For more information about Svelte, Sapper and the template this project builds upon, see the README and documentation for the [Sapper Template](https://github.com/sveltejs/sapper-template) project.

## Local Development
To run local development you need node installed. Initially you need to install the node modules required. So run this from within your project folder after cloning the repository:
```bash
npm install
```
Then to start a local development server simply run:
```bash
npm run dev
```
This will start a local development server on [localhost:3000](http://localhost:3000/). The development server supports auto reloadning, meaning the website should update in real time as you save files when working.

For local development it is recommended to use [VS Code](https://code.visualstudio.com) with the [Svelte for VS Code](https://marketplace.visualstudio.com/items?itemName=svelte.svelte-vscode). The [Svelte Type Checker](https://marketplace.visualstudio.com/items?itemName=halfnelson.svelte-type-checker-vscode), [Svelte Intellisense](https://marketplace.visualstudio.com/items?itemName=ardenivanov.svelte-intellisense) and [Svelte 3 Snippets](https://marketplace.visualstudio.com/items?itemName=fivethree.vscode-svelte-snippets) might also be usefull.


## Publish
To start the final production ready server:
```bash
docker-compose up --build -d
```
This will start the webserver on port [localhost:3000](http://localhost:3000/).


