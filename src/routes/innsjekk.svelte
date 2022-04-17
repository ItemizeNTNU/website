<script context="module">
	import api from '../utils/api';
	export async function preload() {
		let event = (await api.getCheckin('8acacc43-4ba1-4faf-9981-5a57d9b68047', { fetch: this.fetch })).json;
		return { event };
	}
</script>

<script>
	import { user } from '../utils/stores';
	export let event;
	console.log(event);
</script>

<svelte:head>
	<title>Innsjekk</title>
</svelte:head>

<main>
	<h1>Innsjekk</h1>
	<p>
		Lorem ipsum dolor sit amet consectetur adipisicing elit. Commodi, molestias iste laborum quod eius ipsa quaerat dolorem unde error eligendi aspernatur corporis debitis ipsam
		quas atque accusamus animi doloremque aperiam.
	</p>
	<div>
		<ul>
			<li class="card" class:old={new Date(event.end) < new Date()}>
				{#if $user?.roles?.includes('Styret')}
					<div class="buttons" />
				{/if}
				<h3>{event.name} <span class="done">{new Date(event.end) < new Date() ? '[Ferdig]' : ''}</span></h3>
			</li>
		</ul>
	</div>
</main>
