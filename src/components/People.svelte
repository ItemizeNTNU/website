<script>
	export let people;
</script>

<ul>
	{#each people as person}
		<li>
			<b>{person.position}</b>: {person.name}
			<span class="right"><a href="mailto:{person.mail}">{person.mail}</a></span>
		</li>
	{/each}
</ul>

<style>
	li {
		list-style-type: none;
		background-color: rgba(255, 255, 255, 0.1);
		padding: 0.5em;
		margin-bottom: 1em;
		border: 2px solid rgba(0, 0, 0, 0);
		transition: border 0.2s;
	}

	li:hover {
		border: 2px solid rgba(255, 255, 255, 0.05);
		transition: border 0s;
	}

	ul {
		padding: 0;
	}

	.right {
		float: right;
	}
</style>
