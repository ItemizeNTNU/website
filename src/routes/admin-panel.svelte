<script context="module">
	import api from '../utils/api';

	export async function preload(page, session) {
		if (!session.user) {
			this.redirect(302, '/login');
		} else if (!session.user?.roles.includes('Styret')) {
			this.redirect(401, '/');
		}
		const query = 'queryString=*';
		const dbUsers = await api.searchUsers(query, { fetch: this.fetch });
		const dbApplications = await api.getAllApplications({ fetch: this.fetch });
		const dbGroups = await api.getAllGroups({ fetch: this.fetch });
		return { users: dbUsers.json.users, applications: dbApplications.json.applications, groups: dbGroups.json.groups };
	}
</script>

<script>
	import AdminPanelTable from '../components/AdminPanelTable.svelte';
	import date from '../utils/date';
	export let users;
	export let applications;
	export let groups;

	let selectedCols = ['fullName', 'discordName', 'email', 'type'];
	const COLUMNS = {
		fullName: {
			key: 'fullName',
			title: 'Navn',
			value: (v) => v.fullName,
			sortable: true
		},
		discordName: {
			key: 'discordName',
			title: 'Discord-navn',
			value: (v) => v.discordUsername || '',
			sortable: true
		},
		email: {
			key: 'email',
			title: 'E-post',
			value: (v) => v.email,
			sortable: true
		},
		type: {
			key: 'type',
			title: 'Medlemstype',
			value: (v) => v.type || '',
			sortable: true
		}
	};
	const simpleFilter = {
		title: 'Søk etter brukere (navn, discord-navn og epost): ',
		filter: (r, v) => [r.name, r.discordUsername, r.email].find((e) => e?.toLocaleLowerCase().indexOf(v.toLocaleLowerCase()) >= 0)
	};
	const advancedFilter = [
		{ name: 'Navn: ', default: '', filter: (r, v) => r.fullName.toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0 },
		{ name: 'Visningsnavn: ', default: '', filter: (r, v) => r.displayName.toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0 },
		{ name: 'Discord-navn: ', default: '', filter: (r, v) => (r.discordUsername || '').toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0 },
		{ name: 'E-post: ', default: '', filter: (r, v) => r.email.toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0 },
		{ name: 'Registrert før: ', default: '', isDate: true, filter: (r, v) => r.insertInstant <= (new Date(v).getTime() || r.insertInstant) },
		{ name: 'Registrert etter: ', default: '', isDate: true, filter: (r, v) => r.insertInstant >= (new Date(v).getTime() || 0) },
		{ name: 'Medlemstype: ', default: [], values: ['alumni', 'employee', 'student'], filter: (r, v) => v?.includes(r.type) || v?.length == 0 },
		{
			name: 'Gruppe: ',
			default: [],
			values: Object.keys(groups).map((id) => groups[id].name),
			filter: (r, v) => r.groupIds?.find((id) => v?.includes(groups[id].name)) || v?.length == 0
		},
		{
			name: 'Applikasjon: ',
			default: [],
			values: Object.keys(applications).map((id) => applications[id].name),
			filter: (r, v) => r.applicationRoles?.find((app) => v?.includes(applications[app.id].name)) || v?.length == 0
		}
	];

	$: cols = selectedCols.map((key) => COLUMNS[key]);
</script>

<svelte:head>
	<title>Admin panel</title>
</svelte:head>

<main>
	<div class="container">
		<div class="row">
			<div class="main-info">
				<h2>Totalt antall medlemmer: {users.length}</h2>
				<span>Studenter: {users.filter((e) => e.type == 'student').length}</span>
				<span>Alumni: {users.filter((e) => e.type == 'alumni').length}</span>
				<span>Ansatte: {users.filter((e) => e.type == 'employee').length}</span>
			</div>
			<br />
			<AdminPanelTable
				columns={cols}
				rows={users}
				showExpandIcon={true}
				expandRowKey="fullName"
				iconExpand="⌄"
				iconExpanded="⌃"
				simpleFilterOptions={simpleFilter}
				advancedFilterOptions={advancedFilter}
			>
				<div slot="expanded" let:row class="user-info">
					<p><b>Fullt Navn: </b>{row.fullName}</p>
					<p><b>Visningsnavn: </b>{row.displayName}</p>
					<p><b>E-post: </b>{row.email}</p>
					<p><b>Medlemstype: </b>{row.type || ''}</p>
					{#if row.type == 'student' || row.type == 'alumni'}
						<p><b>Studieretning: </b>{row.study.program}</p>
						{#if row.type == 'student'}
							<p><b>Årstrinn: </b>{row.study.year}</p>
						{:else}
							<p><b>Medlemsår: </b>{row.alumni.joinYear}</p>
						{/if}
					{:else if row.type == 'employee'}
						<p><b>Title: </b>{row.employee.title}</p>
						<p />
					{/if}
					<div class="user-roles">
						<h4><b>Grupper</b></h4>
						<hr />
						{#if row?.groupIds}
							<div class="role-info">
								<p><b>Navn:</b></p>
								<p><b>Tilgang til applikasjoner:</b></p>
								{#each row.groupIds as id}
									<p>{groups[id]?.name}</p>
									<p>
										{Object.keys(groups[id]?.roles)
											.map((e) => applications[e].name)
											.join(', ')}
									</p>
								{/each}
							</div>
						{:else}
							<p><i>Ikke medlem av noen grupper</i></p>
						{/if}
					</div>
					<div class="user-roles">
						<h4><b>Applikasjoner</b></h4>
						<hr />
						{#if row?.applicationRoles}
							<div class="role-info">
								<span><b>Navn:</b></span><span><b>Registrert med roller:</b></span>
								{#each row.applicationRoles as application}
									<p>{applications[application.id]?.name}</p>
									<p>{application.roles.join(', ')}</p>
								{/each}
							</div>
						{:else}
							<p><i>Ikke tilgang til noen applikasjoner</i></p>
						{/if}
					</div>
					<p><b>Bruker opprettet: </b>{date.nicePrintDate(new Date(row.insertInstant))}</p>
					<p><b>Sist logget in: </b>{date.nicePrintDate(new Date(row.lastLoginInstant))}</p>
				</div>
			</AdminPanelTable>
		</div>
	</div>
</main>

<style>
	.main-info {
		display: grid;
		grid-template-columns: 40% 20% 20% 20%;
		align-items: top;
		padding: 0.6em;
	}
	.user-info {
		display: grid;
		grid-template-columns: 50% 50%;
		align-items: top;
		padding: 0.6em;
	}
	.user-info * {
		margin: 0.1em 0;
	}
	.user-info p,
	div {
		width: 100%;
	}
	hr {
		display: block;
		height: 1px;
		border: 0;
		width: 70%;
		border-top: 1px solid #ccc;
		margin: 1em 0;
		padding: 0;
	}
	.user-roles {
		padding: 5px;
	}
	.user-roles p {
		padding: 5px;
	}
	.role-info {
		display: grid;
		grid-template-columns: 25% 75%;
		align-items: top;
		margin: 0;
		padding: 0;
	}
	.role-info p {
		width: 100%;
		padding: 0px;
		padding-left: 5px;
	}
</style>
