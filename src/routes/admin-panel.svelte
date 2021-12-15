<script context="module">
	import api from '../utils/api';

	export async function preload(page, session) {
		/*if (!session.user) {
			this.redirect(302, '/login');
		} else if (!session.user?.roles.includes('Styret')) {
			this.redirect(401, '/');
		}*/
		const query = 'queryString=*';
		const dbUsers = [
				{
					id: '423081ed-38de-4644-b781-032689541008',
					email: 'carlarrestad@gmail.com',
					fullName: 'carl aarresstad',
					name: 'baba',
					imageUrl: 'https://itemize.no/logo-512.png',
					type: 'student',
					study: { program: 'digsec', year: 2 },
					insertInstant: 1637599513896,
					lastLoginInstant: 1637599998250,
					groupIds: [ '18a37522-1a9f-4352-9655-e2e918994e03' ],
					applicationIds: [
					'1f526827-6766-4a9a-9f40-2ffb03f543b0',
					'3c219e58-ed0e-4b18-ad48-f4f92793ae32'
					],
					roles: [ 'Styret', 'admin' ]
				},
				{
					id: 'bf1c0c6c-ac9e-4bc5-8b05-c10d84bcc052',
					email: 'kristian2@intveld.no',
					fullName: 'Kristian Dev',
					name: 'kristian-dev',
					imageUrl: 'https://itemize.no/logo-512.png',
					type: 'student',
					study: { program: 'komtek', year: 5 },
					insertInstant: 1631805008977,
					lastLoginInstant: 1631810623865,
					applicationIds: [ '1f526827-6766-4a9a-9f40-2ffb03f543b0' ],
					roles: [ null ]
				},
				{
					id: '54a39eee-a16e-4f69-a7e5-628f25bc7ace',
					email: 'kristian@intveld.no',
					fullName: "Kristian in't Veld",
					name: "Kristian in't Veld",
					imageUrl: 'https://cdn.discordapp.com/avatars/116906482003476486/0265a1d94551f2cfb2cd05baf2259f32.png?size=512',
					discordUsername: 'null#3702',
					isDiscordMember: true,
					insertInstant: 1631619707056,
					lastLoginInstant: 1639506259515,
					applicationIds: [
					'1f526827-6766-4a9a-9f40-2ffb03f543b0',
					'6543e31a-e126-48f7-894d-4dd3fa2ef0f1',
					'3c219e58-ed0e-4b18-ad48-f4f92793ae32'
					],
					roles: [ null, null, 'admin' ]
				},
				{
					id: '529ceecd-f40b-445d-8d3e-5a129753eb13',
					email: 'shirajuki00@gmail.com',
					fullName: 'Jonny Ngo Luong',
					name: 'Shirajuki',
					imageUrl: 'https://cdn.discordapp.com/avatars/181020905642786816/dba16d442079d435e398506171d99644.png?size=512',
					type: 'student',
					study: { program: 'MSIT', year: 1 },
					discordUsername: 'Shirajuki#1892',
					isDiscordMember: true,
					insertInstant: 1631808316883,
					lastLoginInstant: 1635595763288,
					groupIds: [ '18a37522-1a9f-4352-9655-e2e918994e03' ],
					applicationIds: [
					'1f526827-6766-4a9a-9f40-2ffb03f543b0',
					'3c219e58-ed0e-4b18-ad48-f4f92793ae32',
					'6543e31a-e126-48f7-894d-4dd3fa2ef0f1'
					],
					roles: [ 'Styret', 'admin', null ]
				},
				{
					id: 'b8408ac6-f411-47fa-8c09-bb3ae7cfaff3',
					email: 'shirajuki@corax.team',
					fullName: 'jonnytest',
					name: 'jonnytest',
					imageUrl: 'https://itemize.no/logo-512.png',
					type: 'student',
					study: { program: 'MSIT', year: 1 },
					insertInstant: 1634841148427,
					lastLoginInstant: 1634841148427
				},
				{
					id: '48262bbf-6650-48e6-be38-aa4dd4065249',
					email: 'sigurd@varhaugvik.com',
					fullName: 'Sigurds testbruker',
					name: 'sigurd',
					imageUrl: 'https://itemize.no/logo-512.png',
					type: 'student',
					study: { program: 'komtek', year: 1 },
					insertInstant: 1639312749994,
					lastLoginInstant: 1639504333488,
					groupIds: [ '18a37522-1a9f-4352-9655-e2e918994e03' ],
					applicationIds: [
					'3c219e58-ed0e-4b18-ad48-f4f92793ae32',
					'1f526827-6766-4a9a-9f40-2ffb03f543b0'
					],
					roles: [ 'admin', 'Styret' ]
				}
				]
		//await api.searchUsers(query, { fetch: this.fetch });
		return { users: dbUsers/*.json.users*/ };
	}
</script>

<script>
	import AdminPanelTable from '../components/AdminPanelTable.svelte';
	export let users;
	let selectedCols = ['fullName', 'discordName', 'email', 'type', 'roles'];
	let selection = { fullName: "", discordName: "", email: "" };
	const COLUMNS = {
		fullName: {
			key: 'fullName',
			title: 'Navn',
			value: (v) => v.fullName,
			sortable: true,
			searchValue: v => v.fullName,
		},
		discordName: {
			key: 'discordName',
			title: 'Discord-navn',
			value: (v) => v.discordUsername || '',
			sortable: true,
			searchValue: v => v.discordUsername,
		},
		email: {
			key: 'email',
			title: 'E-post',
			value: (v) => v.email,
			sortable: true,
			searchValue: v => v.email,
		},
		type: {
			key: 'type',
			title: 'Medlemstype',
			value: (v) => v.type || '',
			sortable: true
		},
		roles: {
			key: 'roles',
			title: 'Roller',
			value: (v) => v.roles,
			sortable: true
		}
	};
	const filterOptions = {
		'type': ['alumni', 'ansatt', 'student']
	}

	$: cols = selectedCols.map((key) => COLUMNS[key]);

	function handleExpand(event) {
		const row = event.detail.row;
		console.log(row);
		//const operation = row.$expanded ? "open" : "close";
	}
</script>

<svelte:head>
	<title>Admin panel</title>
</svelte:head>

<main>
	<div class="container">
		<div class="row">
			<h2>Expand row example 2</h2>

			<p>
				Only 1 row can be expanded at a time<br />
				Console logs selection change
			</p>

			<AdminPanelTable
				columns={cols}
				rows={users}
				classNameTable="table"
				classNameThead="table-info"
				showExpandIcon={true}
				expandSingle={true}
				expandRowKey="fullName"
				iconExpand="⌄"
				iconExpanded="⌃"
				filterOptions={filterOptions}
				on:clickExpand={handleExpand}
				bind:filterSelections={selection}
			>
				<div slot="expanded" let:row class="text-center">
					{row.county}, {row.state}<br />
					{row.country}
				</div>
			</AdminPanelTable>
		</div>
	</div>
</main>
